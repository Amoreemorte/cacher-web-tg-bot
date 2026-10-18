package llm

import (
	"context"
	"fmt"
)

// MockLLM — заглушка LLMClient для тестов и разработки без реального API.
// Возвращает заранее заданный ответ или ошибку, если установлен Err.
type MockLLM struct {
	Response *LLMResponse
	Err      error
	CallCount int
	LastUserID int64
	LastText   string
}

// ProcessText реализует интерфейс LLMClient.
func (m *MockLLM) ProcessText(ctx context.Context, userID int64, text string) (*LLMResponse, error) {
	m.CallCount++
	m.LastUserID = userID
	m.LastText = text

	if m.Err != nil {
		return nil, m.Err
	}

	if m.Response == nil {
		return nil, fmt.Errorf("mock response not set")
	}

	// Копируем ответ, чтобы не менять оригинал
	resp := *m.Response
	resp.UserID = userID
	return &resp, nil
}