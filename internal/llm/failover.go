package llm

import (
	"context"
	"fmt"
	"log"
	"time"
)

// FailoverClient пытается отправить запрос в основной LLM, при ошибке — в резервный.
// Если оба недоступны — возвращает ErrLLMUnavailable и caller может сохранить задачу в очередь.
type FailoverClient struct {
	primary   LLMClient
	backup    LLMClient
	maxRetries int
	retryDelay time.Duration
}

// NewFailoverClient создаёт клиент с фейловером.
func NewFailoverClient(primary, backup LLMClient, maxRetries int, retryDelay time.Duration) *FailoverClient {
	return &FailoverClient{
		primary:    primary,
		backup:     backup,
		maxRetries: maxRetries,
		retryDelay: retryDelay,
	}
}

// ProcessText отправляет запрос сначала в primary, при ошибке — в backup.
func (f *FailoverClient) ProcessText(ctx context.Context, userID int64, text string) (*LLMResponse, error) {
	var lastErr error

	// Попытки в основном провайдере
	for i := 0; i <= f.maxRetries; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(f.retryDelay):
			}
		}

		resp, err := f.primary.ProcessText(ctx, userID, text)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		log.Printf("primary LLM attempt %d failed: %v", i+1, err)

		if IsLLMUnavailable(err) {
			break
		}
	}

	// Если есть резервный — пробуем его
	if f.backup != nil {
		resp, err := f.backup.ProcessText(ctx, userID, text)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		log.Printf("backup LLM failed: %v", err)
	}

	return nil, fmt.Errorf("%w: %v", ErrLLMUnavailable, lastErr)
}