package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPClient_ProcessText_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Authorization Bearer test-key, got %s", r.Header.Get("Authorization"))
		}

		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
			return
		}
		if req.Model != "test-model" {
			t.Errorf("expected model test-model, got %s", req.Model)
		}
		if req.MaxTokens != 512 {
			t.Errorf("expected max_tokens 512, got %d", req.MaxTokens)
		}
		if len(req.Messages) != 2 {
			t.Errorf("expected 2 messages, got %d", len(req.Messages))
		}
		if req.Messages[0].Role != "system" {
			t.Errorf("expected first message role system, got %s", req.Messages[0].Role)
		}
		if req.Messages[1].Role != "user" {
			t.Errorf("expected second message role user, got %s", req.Messages[1].Role)
		}

		resp := chatResponse{
			ID: "test-id",
			Choices: []chatChoice{
				{
					Index: 0,
					Message: chatMessage{
						Role:    "assistant",
						Content: `{"action":"add","user_id":12345,"amount":350,"category":"еда","spent_at":"2026-10-18T10:00:00Z","comment":"кофе","expense_id":null}`,
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "test-key", "test-model", 512, 5*time.Second)

	resp, err := client.ProcessText(context.Background(), 12345, "кофе 350")
	if err != nil {
		t.Fatalf("ProcessText failed: %v", err)
	}
	if resp.Action != "add" {
		t.Errorf("action = %q, want add", resp.Action)
	}
	if resp.Amount != 350 {
		t.Errorf("amount = %v, want 350", resp.Amount)
	}
	if resp.Category != "еда" {
		t.Errorf("category = %q, want еда", resp.Category)
	}
	if resp.Comment != "кофе" {
		t.Errorf("comment = %q, want кофе", resp.Comment)
	}
}

func TestHTTPClient_ProcessText_MarkdownWrapped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse{
			ID: "test-id",
			Choices: []chatChoice{
				{
					Index: 0,
					Message: chatMessage{
						Role:    "assistant",
						Content: "```json\n{\"action\":\"delete\",\"user_id\":1,\"amount\":0,\"category\":\"транспорт\",\"spent_at\":\"\",\"comment\":\"\",\"expense_id\":42}\n```",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "", "test-model", 512, 5*time.Second)

	resp, err := client.ProcessText(context.Background(), 1, "удали такси")
	if err != nil {
		t.Fatalf("ProcessText failed: %v", err)
	}
	if resp.Action != "delete" {
		t.Errorf("action = %q, want delete", resp.Action)
	}
	if resp.ExpenseID == nil || *resp.ExpenseID != 42 {
		t.Errorf("expense_id = %v, want 42", resp.ExpenseID)
	}
}

func TestHTTPClient_ProcessText_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"internal error","type":"server_error"}}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "", "test-model", 512, 5*time.Second)

	_, err := client.ProcessText(context.Background(), 1, "кофе 350")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	serverErr, ok := err.(*LLMServerError)
	if !ok {
		t.Fatalf("expected LLMServerError, got %T: %v", err, err)
	}
	if serverErr.StatusCode != 500 {
		t.Errorf("status code = %d, want 500", serverErr.StatusCode)
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain json", `{"a":1}`, `{"a":1}`},
		{"markdown wrapped", "```json\n{\"a\":1}\n```", `{"a":1}`},
		{"markdown without lang", "```\n{\"a\":1}\n```", `{"a":1}`},
		{"with surrounding text", `Вот ответ: {"a":1} готово`, `{"a":1}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSON(tt.input)
			if got != tt.want {
				t.Errorf("extractJSON(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
