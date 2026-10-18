package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"expense-bot/internal/domain"
	"expense-bot/internal/llm"
	"expense-bot/internal/repository/sqlite"
)

// newTestService создаёт сервис с временной SQLite БД и моком LLM.
func newTestService(t *testing.T) (*ExpenseService, *llm.MockLLM, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_service.db")

	repo, err := sqlite.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}

	mockLLM := &llm.MockLLM{}
	svc := NewExpenseService(mockLLM, repo, repo)

	cleanup := func() {
		repo.Close()
		os.Remove(dbPath)
	}

	return svc, mockLLM, cleanup
}

func TestExpenseService_AddExpense(t *testing.T) {
	ctx := context.Background()
	svc, mockLLM, cleanup := newTestService(t)
	defer cleanup()

	userID := int64(12345)

	mockLLM.Response = &llm.LLMResponse{
		Action:    "add",
		Amount:    350,
		Category:  "еда",
		SpentAt:   time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Comment:   "кофе",
		ExpenseID: nil,
	}

	reply, err := svc.ProcessMessage(ctx, userID, "кофе 350")
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	if mockLLM.CallCount != 1 {
		t.Errorf("expected 1 LLM call, got %d", mockLLM.CallCount)
	}
	if mockLLM.LastUserID != userID {
		t.Errorf("LLM userID = %d, want %d", mockLLM.LastUserID, userID)
	}
	if mockLLM.LastText != "кофе 350" {
		t.Errorf("LLM text = %q, want кофе 350", mockLLM.LastText)
	}

	expenses, err := svc.repo.GetByUserID(ctx, userID)
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
	_ = reply
}

func TestExpenseService_LLMUnavailable(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := newTestService(t)
	defer cleanup()
	userID := int64(12345)
	mockLLM := &llm.MockLLM{Err: llm.ErrLLMUnavailable}
	svc.llmClient = mockLLM
	reply, err := svc.ProcessMessage(ctx, userID, "кофе 350")
	if err != nil {
		t.Fatalf("ProcessMessage should not return error when LLM unavailable, got: %v", err)
	}
	if reply != "LLM не отвечает, попробуйте позже" {
		t.Errorf("reply = %q, want LLM unavailable message", reply)
	}
	expenses, err := svc.repo.GetByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("GetByUserID failed: %v", err)
	}
	if len(expenses) != 0 {
		t.Errorf("expected 0 expenses when LLM unavailable, got %d", len(expenses))
	}
}

func TestExpenseService_UpdateExpense(t *testing.T) {
	ctx := context.Background()
	svc, mockLLM, cleanup := newTestService(t)
	defer cleanup()
	userID := int64(12345)
	expense := &domain.Expense{
		UserID:    userID,
		Amount:    350,
		Category:  "еда",
		SpentAt:   time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC),
		Comment:   "кофе",
		CreatedAt: time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC),
	}
	if err := svc.repo.Create(ctx, expense); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	expenseID := expense.ID
	mockLLM.Response = &llm.LLMResponse{
		Action:     "update",
		Amount:     450,
		Category:   "развлечения",
		SpentAt:    time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Comment:    "обновлённый комментарий",
		ExpenseID:  &expenseID,
	}

	if _, err := svc.ProcessMessage(ctx, userID, "последнее было не 350, а 450"); err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	got, err := svc.repo.GetByID(ctx, expenseID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Amount != 450 {
		t.Errorf("amount after update = %v, want 450", got.Amount)
	}
	if got.Category != "развлечения" {
		t.Errorf("category after update = %q, want развлечения", got.Category)
	}
	if got.Comment != "обновлённый комментарий" {
		t.Errorf("comment after update = %q, want обновлённый комментарий", got.Comment)
	}
}

func TestExpenseService_DeleteExpense(t *testing.T) {
	ctx := context.Background()
	svc, mockLLM, cleanup := newTestService(t)
	defer cleanup()

	userID := int64(12345)

	expense := &domain.Expense{
		UserID:    userID,
		Amount:    350,
		Category:  "еда",
		SpentAt:   time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC),
		Comment:   "кофе",
		CreatedAt: time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC),
	}
	if err := svc.repo.Create(ctx, expense); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	expenseID := expense.ID
	mockLLM.Response = &llm.LLMResponse{
		Action:     "delete",
		ExpenseID:  &expenseID,
	}

	if _, err := svc.ProcessMessage(ctx, userID, "удали кофе"); err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	got, err := svc.repo.GetByID(ctx, expenseID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after delete, got expense ID %d", got.ID)
	}
}

func TestExpenseService_CannotModifyOthersExpense(t *testing.T) {
	ctx := context.Background()
	svc, mockLLM, cleanup := newTestService(t)
	defer cleanup()

	userID := int64(12345)
	otherUserID := int64(99999)

	expense := &domain.Expense{
		UserID:    otherUserID,
		Amount:    350,
		Category:  "еда",
		SpentAt:   time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC),
		Comment:   "чужая трата",
		CreatedAt: time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC),
	}
	if err := svc.repo.Create(ctx, expense); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	expenseID := expense.ID
	mockLLM.Response = &llm.LLMResponse{
		Action:     "update",
		Amount:     999,
		ExpenseID:  &expenseID,
	}

	reply, err := svc.ProcessMessage(ctx, userID, "измени мою трату")
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}
	if reply != "Чужие траты редактировать нельзя." {
		t.Errorf("reply = %q, want forbidden message", reply)
	}

	got, err := svc.repo.GetByID(ctx, expenseID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Amount != 350 {
		t.Errorf("amount should not change, got %v", got.Amount)
	}
}

func TestExpenseService_ListExpenses(t *testing.T) {
	ctx := context.Background()
	svc, mockLLM, cleanup := newTestService(t)
	defer cleanup()

	userID := int64(12345)

	now := time.Now().UTC()
	expenses := []*domain.Expense{
		{UserID: userID, Amount: 100, Category: "еда", SpentAt: now.Add(-2 * time.Hour), Comment: "завтрак", CreatedAt: now, UpdatedAt: now},
		{UserID: userID, Amount: 200, Category: "транспорт", SpentAt: now.Add(-1 * time.Hour), Comment: "такси", CreatedAt: now, UpdatedAt: now},
	}
	for _, e := range expenses {
		if err := svc.repo.Create(ctx, e); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	mockLLM.Response = &llm.LLMResponse{
		Action:   "list",
		Category: "еда",
	}

	reply, err := svc.ProcessMessage(ctx, userID, "покажи траты на еду")
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}
	if reply == "" {
		t.Error("expected non-empty reply for list")
	}
	_ = mockLLM
}

func TestExpenseService_UnknownAction(t *testing.T) {
	ctx := context.Background()
	svc, mockLLM, cleanup := newTestService(t)
	defer cleanup()

	userID := int64(12345)

	mockLLM.Response = &llm.LLMResponse{
		Action: "unknown_action",
	}

	reply, err := svc.ProcessMessage(ctx, userID, "какой-то текст")
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}
	if reply != "Не понял запрос. Попробуйте написать иначе (например: «кофе 350»)." {
		t.Errorf("reply = %q, want unknown action message", reply)
	}
	_ = mockLLM
}

func TestExpenseService_AddExpense_MidnightFallbackToNow(t *testing.T) {
	ctx := context.Background()
	svc, mockLLM, cleanup := newTestService(t)
	defer cleanup()
	userID := int64(12345)

	// LLM прислала полночь сегодняшнего дня (ошибка модели для «сегодня»)
	now := time.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	mockLLM.Response = &llm.LLMResponse{
		Action:   "add",
		Amount:   350,
		Category: "еда",
		SpentAt:  midnight.Format(time.RFC3339),
		Comment:  "семечки",
	}

	if _, err := svc.ProcessMessage(ctx, userID, "семечки 350"); err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	expenses, err := svc.repo.GetByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("GetByUserID failed: %v", err)
	}
	if len(expenses) != 1 {
		t.Fatalf("expected 1 expense, got %d", len(expenses))
	}

	got := expenses[0].SpentAt
	if got.Hour() == 0 && got.Minute() == 0 && got.Second() == 0 {
		t.Errorf("spent_at should be replaced with current time, got midnight %v", got)
	}
}

func TestExpenseService_AddExpense_OldDateClamped(t *testing.T) {
	ctx := context.Background()
	svc, mockLLM, cleanup := newTestService(t)
	defer cleanup()
	userID := int64(12345)

	// LLM галлюцинировала дату полтора года назад (как в переписке: 23.05.2024)
	mockLLM.Response = &llm.LLMResponse{
		Action:   "add",
		Amount:   350,
		Category: "еда",
		SpentAt:  "2024-05-23T12:00:00Z",
		Comment:  "семечки",
	}

	if _, err := svc.ProcessMessage(ctx, userID, "семечки 350"); err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	expenses, err := svc.repo.GetByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("GetByUserID failed: %v", err)
	}
	if len(expenses) != 1 {
		t.Fatalf("expected 1 expense, got %d", len(expenses))
	}

	got := expenses[0].SpentAt
	if time.Since(got) > time.Minute {
		t.Errorf("spent_at should be clamped to now, got %v (clamped from 2024)", got)
	}
}

func TestExpenseService_DeletePassesRecentExpensesContext(t *testing.T) {
	ctx := context.Background()
	svc, mockLLM, cleanup := newTestService(t)
	defer cleanup()
	userID := int64(12345)

	// В БД уже есть трата
	expense := &domain.Expense{
		UserID:    userID,
		Amount:    350,
		Category:  "еда",
		SpentAt:   time.Now().UTC(),
		Comment:   "семечки",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := svc.repo.Create(ctx, expense); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Модель выбирает expense_id из переданного контекста
	expenseID := expense.ID
	mockLLM.Response = &llm.LLMResponse{
		Action:    "delete",
		ExpenseID: &expenseID,
	}

	if _, err := svc.ProcessMessage(ctx, userID, "удали семечки"); err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	// LLM должен получить список последних трат с id
	if !strings.Contains(mockLLM.LastText, "Последние траты") || !strings.Contains(mockLLM.LastText, "id=") {
		t.Errorf("LLM text should contain recent expenses block with ids, got: %q", mockLLM.LastText)
	}

	// Трата удалена
	got, err := svc.repo.GetByID(ctx, expenseID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected expense to be deleted, got ID %d", got.ID)
	}
}
