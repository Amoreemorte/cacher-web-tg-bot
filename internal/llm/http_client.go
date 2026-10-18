package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const systemPrompt = `Ты — ассистент учёта семейных расходов. Пользователь пишет тебе запрос на русском языке. Твоя задача — определить действие и вернуть СТРОГО JSON без markdown-обёртки, без пояснений, только JSON.

Формат ответа (все поля обязательны, кроме expense_id и comment):
{
  "action": "add" | "update" | "delete" | "list",
  "user_id": число (передаётся в запросе, повтори его как есть),
  "amount": число (сумма траты, для add/update; для list/delete можно 0),
  "category": строка из списка: "еда", "транспорт", "жильё", "развлечения", "здоровье", "покупки", "другое",
  "spent_at": строка в формате RFC3339 с точностью до секунд (например "2026-10-18T22:26:00+03:00"):
    - для add БЕЗ указания даты в запросе — ТЕКУЩИЕ дата и время из строки «Текущие дата и время» в сообщении (не полночь, не выдумывай!);
    - для add с «сегодня» — ТЕКУЩИЕ дата и время из сообщения;
    - для «вчера» — вчерашняя дата (время можно любое);
    - для list с «в этом месяце» — начало текущего месяца, с «за неделю» — начало текущей недели;
    - для list БЕЗ указания периода («покажи все траты») — пустая строка "";
    - для update/delete без даты — пустая строка "",
  "comment": строка (необязательный комментарий или пустая строка),
  "expense_id": число или null:
    - для update/delete ОБЯЗАТЕЛЬНО выбери ID из блока «Последние траты пользователя» в сообщении (поле id=...);
    - если пользователь говорит «последнюю/последнее/последний» — бери самую новую (первую в списке);
    - если подходящей траты в списке нет или блока нет — верни null;
    - для add и list всегда null
}

Примеры:
- «кофе 350» → {"action":"add","user_id":123,"amount":350,"category":"еда","spent_at":"2026-10-18T22:26:00+03:00","comment":"кофе","expense_id":null}
- «такси 1200 вчера» → {"action":"add","user_id":123,"amount":1200,"category":"транспорт","spent_at":"2026-10-17T18:00:00+03:00","comment":"такси","expense_id":null}
- «покажи траты на еду в этом месяце» → {"action":"list","user_id":123,"amount":0,"category":"еда","spent_at":"2026-10-01T00:00:00Z","comment":"","expense_id":null}
- «покажи все траты» → {"action":"list","user_id":123,"amount":0,"category":"","spent_at":"","comment":"","expense_id":null}
- «последнее было не 350, а 450» → {"action":"update","user_id":123,"amount":450,"category":"еда","spent_at":"","comment":"","expense_id":42} (42 — id из блока последних трат)
- «удали семечки» → {"action":"delete","user_id":123,"amount":0,"category":"еда","spent_at":"","comment":"","expense_id":42} (42 — id из блока последних трат)

Если запрос непонятен или не относится к расходам, верни: {"action":"unknown","user_id":123,"amount":0,"category":"другое","spent_at":"","comment":"","expense_id":null}`

// HTTPClient реализует LLMClient через OpenAI-совместимый Chat Completions API.
type HTTPClient struct {
	client    *http.Client
	url       string
	apiKey    string
	model     string
	maxTokens int
}

// NewHTTPClient создаёт HTTP-клиент для LLM в формате OpenAI Chat Completions.
// maxTokens ограничивает размер ответа — важно для провайдеров с лимитом
// output-токенов в минуту (например, Groq OTPM): незаданный max_tokens
// может приводить к 429 даже при коротком фактическом ответе.
func NewHTTPClient(url, apiKey, model string, maxTokens int, timeout time.Duration) *HTTPClient {
	if maxTokens <= 0 {
		maxTokens = 512
	}
	return &HTTPClient{
		client:    &http.Client{Timeout: timeout},
		url:       url,
		apiKey:    apiKey,
		model:     model,
		maxTokens: maxTokens,
	}
}

// chatMessage — элемент диалога в формате OpenAI.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest — тело запроса к OpenAI-совместимому API.
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

// chatChoice — один вариант ответа.
type chatChoice struct {
	Index   int         `json:"index"`
	Message chatMessage `json:"message"`
}

// chatResponse — ответ OpenAI-совместимого API.
type chatResponse struct {
	ID      string       `json:"id"`
	Choices []chatChoice `json:"choices"`
	Error   *chatError   `json:"error,omitempty"`
}

type chatError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// ProcessText отправляет текст в LLM и парсит структурированный ответ.
func (c *HTTPClient) ProcessText(ctx context.Context, userID int64, text string) (*LLMResponse, error) {
	userContent := fmt.Sprintf(
		"user_id: %d\nТекущие дата и время (используй их как «сейчас», не выдумывай дату): %s\n\n%s",
		userID, time.Now().Format(time.RFC3339), text,
	)

	reqBody := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userContent},
		},
		Temperature: 0.1,
		MaxTokens:   c.maxTokens,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, &LLMTimeoutError{Duration: c.client.Timeout}
		}
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &LLMServerError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("unmarshal chat response: %w", err)
	}

	if chatResp.Error != nil {
		return nil, &LLMServerError{StatusCode: resp.StatusCode, Body: chatResp.Error.Message}
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("LLM returned no choices")
	}

	content := chatResp.Choices[0].Message.Content
	jsonStr := extractJSON(content)

	var llmResp LLMResponse
	if err := json.Unmarshal([]byte(jsonStr), &llmResp); err != nil {
		return nil, fmt.Errorf("unmarshal LLM JSON (extracted from content): %w (extracted: %s)", err, jsonStr)
	}

	return &llmResp, nil
}

// extractJSON извлекает JSON-объект из текста ответа LLM.
// Убирает markdown-обёртку ```json ... ``` и находит первый {...}.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)

	// Убираем markdown-обёртку ```json ... ``` или ``` ... ```
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) >= 2 {
			lines = lines[1:] // убираем ```json
		}
		if len(lines) >= 1 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
			lines = lines[:len(lines)-1] // убираем закрывающую ```
		}
		s = strings.TrimSpace(strings.Join(lines, "\n"))
	}

	// Находим первый { и последний }
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		s = s[start : end+1]
	}

	return s
}