package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"expense-bot/internal/llm"
	"expense-bot/internal/repository"
	"expense-bot/internal/repository/sqlite"
)

func newTestProcessor(t *testing.T) (*PendingTaskProcessor, *llm.MockLLM, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_processor.db")
	repo, err := sqlite.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	mockLLM := &llm.MockLLM{}
	svc := NewExpenseService(mockLLM, repo, repo)
	processor := NewPendingTaskProcessor(mockLLM, repo, repo, svc, 3, time.Second)
	cleanup := func() {
		repo.Close()
		os.Remove(dbPath)
	}
	return processor, mockLLM, cleanup
}

func TestPendingTaskProcessor_RetriesAndSucceeds(t *testing.T) {
	ctx := context.Background()
	processor, mockLLM, cleanup := newTestProcessor(t)
	defer cleanup()

	userID := int64(12345)

	// Создаём отложенную задачу (как если бы LLM была недоступна ранее)
	task := &repository.PendingTask{
		UserID:     userID,
		RawText:    "кофе 350",
		Attempts:   0,
		NextAttempt: time.Now().UTC(),
		CreatedAt:  time.Now().UTC(),
	}
	if err := processor.pendingRepo.CreatePendingTask(ctx, task); err != nil {
		t.Fatalf("CreatePendingTask failed: %v", err)
	}

	// Теперь LLM доступна — процессор должен выполнить задачу
	mockLLM.Response = &llm.LLMResponse{
		Action:   "add",
		Amount:   350,
		Category: "еда",
		Comment:  "кофе",
	}

	if err := processor.ProcessPending(ctx); err != nil {
		t.Fatalf("ProcessPending failed: %v", err)
	}

	// Проверяем, что трата добавилась в БД
	expenses, err := processor.expenseRepo.GetByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("GetByUserID failed: %v", err)
	}
	if len(expenses) != 1 {
		t.Fatalf("expected 1 expense, got %d", len(expenses))
	}
	if expenses[0].Amount != 350 {
		t.Errorf("amount = %v, want 350", expenses[0].Amount)
	}
	if expenses[0].Category != "еда" {
		t.Errorf("category = %q, want еда", expenses[0].Category)
	}

	// Проверяем, что задача удалена из очереди
	tasks, err := processor.pendingRepo.ListDuePendingTasks(ctx, time.Now())
	if err != nil {
		t.Fatalf("ListDuePendingTasks failed: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 pending tasks, got %d", len(tasks))
	}
}

func TestPendingTaskProcessor_ExceedsMaxAttempts(t *testing.T) {
	ctx := context.Background()
	processor, mockLLM, cleanup := newTestProcessor(t)
	defer cleanup()

	userID := int64(12345)

	// LLM остаётся недоступной
	mockLLM.Err = llm.ErrLLMUnavailable

	// Создаём задачу, которая уже близка к лимиту попыток
	task := &repository.PendingTask{
		UserID:     userID,
		RawText:    "кофе 350",
		Attempts:   4, // maxAttempts = 3, так что превышает
		NextAttempt: time.Now().UTC(),
		CreatedAt:  time.Now().UTC(),
	}
	if err := processor.pendingRepo.CreatePendingTask(ctx, task); err != nil {
		t.Fatalf("CreatePendingTask failed: %v", err)
	}

	if err := processor.ProcessPending(ctx); err != nil {
		t.Fatalf("ProcessPending failed: %v", err)
	}

	// Проверяем, что задача удалена (превышен лимит попыток)
	tasks, err := processor.pendingRepo.ListDuePendingTasks(ctx, time.Now())
	if err != nil {
		t.Fatalf("ListDuePendingTasks failed: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 pending tasks after max attempts, got %d", len(tasks))
	}

	// Проверяем, что трата не добавилась (LLM была недоступна)
	expenses, err := processor.expenseRepo.GetByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("GetByUserID failed: %v", err)
	}
	if len(expenses) != 0 {
		t.Errorf("expected 0 expenses, got %d", len(expenses))
	}
}