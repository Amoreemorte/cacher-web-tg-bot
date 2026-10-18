package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPClient реализует LLMClient через HTTP-запросы к произвольному эндпоинту.
type HTTPClient struct {
	client  *http.Client
	url     string
	apiKey  string
}

// NewHTTPClient создаёт HTTP-клиент для LLM.
func NewHTTPClient(url, apiKey string, timeout time.Duration) *HTTPClient {
	return &HTTPClient{
		client: &http.Client{Timeout: timeout},
		url:    url,
		apiKey: apiKey,
	}
}

// ProcessText отправляет текст в LLM и парсит ответ в LLMResponse.
func (c *HTTPClient) ProcessText(ctx context.Context, userID int64, text string) (*LLMResponse, error) {
	// Формируем запрос к LLM. Формат тела зависит от провайдера;
	// здесь используется общий формат с полями user_id и text.
	requestBody := map[string]interface{}{
		"user_id": userID,
		"text":    text,
	}

	data, err := json.Marshal(requestBody)
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

	var llmResp LLMResponse
	if err := json.Unmarshal(body, &llmResp); err != nil {
		// Если LLM вернул не JSON с нашими полями, пробуем распарсить как общий ответ
		var raw map[string]interface{}
		if unmarshalErr := json.Unmarshal(body, &raw); unmarshalErr != nil {
			return nil, fmt.Errorf("unmarshal LLM response: %w", err)
		}
		return mapToResponse(raw)
	}

	return &llmResp, nil
}

// mapToResponse пытается привести произвольный JSON-ответ LLM к LLMResponse.
func mapToResponse(raw map[string]interface{}) (*LLMResponse, error) {
	resp := &LLMResponse{}

	if v, ok := raw["action"].(string); ok {
		resp.Action = v
	}
	if v, ok := raw["user_id"].(float64); ok {
		resp.UserID = int64(v)
	}
	if v, ok := raw["amount"].(float64); ok {
		resp.Amount = v
	}
	if v, ok := raw["category"].(string); ok {
		resp.Category = v
	}
	if v, ok := raw["spent_at"].(string); ok {
		resp.SpentAt = v
	}
	if v, ok := raw["comment"].(string); ok {
		resp.Comment = v
	}
	if v, ok := raw["expense_id"]; ok && v != nil {
		if f, ok := v.(float64); ok {
			id := int64(f)
			resp.ExpenseID = &id
		}
	}

	return resp, nil
}