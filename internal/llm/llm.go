package llm

import (
	"context"
	"errors"
	"time"
)

// LLMResponse — структура, которую возвращает LLM после обработки текста пользователя.
type LLMResponse struct {
	Action     string  `json:"action"`      // add | update | delete | list
	UserID     int64   `json:"user_id"`     // Telegram ID пользователя
	Amount     float64 `json:"amount"`      // сумма (для add/update)
	Category   string  `json:"category"`    // категория (для add/update)
	SpentAt    string  `json:"spent_at"`    // дата траты ISO 8601 (для add/update)
	Comment    string  `json:"comment"`     // комментарий (опционально)
	ExpenseID  *int64  `json:"expense_id"`  // ID записи (для update/delete)
}

// LLMClient определяет интерфейс для взаимодействия с языковой моделью.
// Реализацию можно заменить на мок в тестах или на другого провайдера.
type LLMClient interface {
	// ProcessText отправляет текст пользователя в LLM и возвращает структурированный ответ.
	ProcessText(ctx context.Context, userID int64, text string) (*LLMResponse, error)
}

// ErrLLMUnavailable возвращается, когда все LLM-провайдеры недоступны.
var ErrLLMUnavailable = errors.New("LLM не отвечает, попробуйте позже")

// LLMTimeoutError возвращается при превышении таймаута запроса к LLM.
type LLMTimeoutError struct {
	Duration time.Duration
}

func (e *LLMTimeoutError) Error() string {
	return "LLM request timeout after " + e.Duration.String()
}

// LLMServerError возвращается при 4xx/5xx ответе от LLM.
type LLMServerError struct {
	StatusCode int
	Body       string
}

func (e *LLMServerError) Error() string {
	return "LLM server error: status=" + itoa(e.StatusCode) + " body=" + e.Body
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// IsLLMUnavailable проверяет, является ли ошибка «LLM недоступен».
func IsLLMUnavailable(err error) bool {
	return errors.Is(err, ErrLLMUnavailable)
}