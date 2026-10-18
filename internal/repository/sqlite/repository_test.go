package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"expense-bot/internal/domain"
	_ "modernc.org/sqlite"
)

// newTestRepo создаёт временную SQLite БД для тестов.
func newTestRepo(t *testing.T) (*SQLiteRepository, string) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_expenses.db")

	repo, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}

	t.Cleanup(func() {
		repo.Close()
		os.Remove(dbPath)
	})

	return repo, dbPath
}

func TestExpenseRepository_CreateAndGetByID(t *testing.T) {
	ctx := context.Background()
	repo, _ := newTestRepo(t)

	expense := &domain.Expense{
		UserID:    12345,
		Amount:    350.00,
		Category:  "еда",
		SpentAt:   time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC),
		Comment:   "кофе",
		CreatedAt: time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC),
	}

	if err := repo.Create(ctx, expense); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if expense.ID == 0 {
		t.Fatal("expected ID to be set after Create")
	}

	got, err := repo.GetByID(ctx, expense.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetByID returned nil")
	}

	if got.UserID != expense.UserID {
		t.Errorf("UserID = %d, want %d", got.UserID, expense.UserID)
	}
	if got.Amount != expense.Amount {
		t.Errorf("Amount = %v, want %v", got.Amount, expense.Amount)
	}
	if got.Category != expense.Category {
		t.Errorf("Category = %q, want %q", got.Category, expense.Category)
	}
	if got.Comment != expense.Comment {
		t.Errorf("Comment = %q, want %q", got.Comment, expense.Comment)
	}
}

func TestExpenseRepository_GetByUserID(t *testing.T) {
	ctx := context.Background()
	repo, _ := newTestRepo(t)

	now := time.Now().UTC()
	expenses := []*domain.Expense{
		{UserID: 1, Amount: 100, Category: "еда", SpentAt: now.Add(-2 * time.Hour), Comment: "завтрак", CreatedAt: now, UpdatedAt: now},
		{UserID: 1, Amount: 200, Category: "транспорт", SpentAt: now.Add(-1 * time.Hour), Comment: "такси", CreatedAt: now, UpdatedAt: now},
		{UserID: 2, Amount: 50, Category: "еда", SpentAt: now, Comment: "обед", CreatedAt: now, UpdatedAt: now},
	}

	for _, e := range expenses {
		if err := repo.Create(ctx, e); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	user1Expenses, err := repo.GetByUserID(ctx, 1)
	if err != nil {
		t.Fatalf("GetByUserID failed: %v", err)
	}
	if len(user1Expenses) != 2 {
		t.Errorf("expected 2 expenses for user 1, got %d", len(user1Expenses))
	}

	user2Expenses, err := repo.GetByUserID(ctx, 2)
	if err != nil {
		t.Fatalf("GetByUserID failed: %v", err)
	}
	if len(user2Expenses) != 1 {
		t.Errorf("expected 1 expense for user 2, got %d", len(user2Expenses))
	}
}

func TestExpenseRepository_Update(t *testing.T) {
	ctx := context.Background()
	repo, _ := newTestRepo(t)

	expense := &domain.Expense{
		UserID:    1,
		Amount:    100,
		Category:  "еда",
		SpentAt:   time.Now().UTC(),
		Comment:   "старый комментарий",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := repo.Create(ctx, expense); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	expense.Amount = 450
	expense.Category = "развлечения"
	expense.Comment = "новый комментарий"
	expense.UpdatedAt = time.Now().UTC()

	if err := repo.Update(ctx, expense); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, err := repo.GetByID(ctx, expense.ID)
	if err != nil {
		t.Fatalf("GetByID after Update failed: %v", err)
	}
	if got.Amount != 450 {
		t.Errorf("Amount after update = %v, want 450", got.Amount)
	}
	if got.Category != "развлечения" {
		t.Errorf("Category after update = %q, want развлечения", got.Category)
	}
	if got.Comment != "новый комментарий" {
		t.Errorf("Comment after update = %q, want новый комментарий", got.Comment)
	}
}

func TestExpenseRepository_Delete(t *testing.T) {
	ctx := context.Background()
	repo, _ := newTestRepo(t)

	expense := &domain.Expense{
		UserID:    1,
		Amount:    100,
		Category:  "еда",
		SpentAt:   time.Now().UTC(),
		Comment:   "to delete",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := repo.Create(ctx, expense); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := repo.Delete(ctx, expense.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	got, err := repo.GetByID(ctx, expense.ID)
	if err != nil {
		t.Fatalf("GetByID after Delete failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after Delete, got expense with ID %d", got.ID)
	}
}

func TestExpenseRepository_GetAll_Filter(t *testing.T) {
	ctx := context.Background()
	repo, _ := newTestRepo(t)

	now := time.Now().UTC()
	expenses := []*domain.Expense{
		{UserID: 1, Amount: 100, Category: "еда", SpentAt: now.AddDate(0, 0, -5), Comment: "понедельник", CreatedAt: now, UpdatedAt: now},
		{UserID: 1, Amount: 200, Category: "транспорт", SpentAt: now.AddDate(0, 0, -3), Comment: "среда", CreatedAt: now, UpdatedAt: now},
		{UserID: 1, Amount: 300, Category: "еда", SpentAt: now.AddDate(0, 0, -1), Comment: "пятница", CreatedAt: now, UpdatedAt: now},
		{UserID: 2, Amount: 400, Category: "еда", SpentAt: now, Comment: "user2", CreatedAt: now, UpdatedAt: now},
	}

	for _, e := range expenses {
		if err := repo.Create(ctx, e); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	userID := int64(1)
	all, err := repo.GetAll(ctx, domain.ExpenseFilter{UserID: &userID})
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 expenses for user 1, got %d", len(all))
	}

	cat := "еда"
	allFood, err := repo.GetAll(ctx, domain.ExpenseFilter{Category: &cat})
	if err != nil {
		t.Fatalf("GetAll by category failed: %v", err)
	}
	if len(allFood) != 3 {
		t.Errorf("expected 3 food expenses, got %d", len(allFood))
	}

	from := now.AddDate(0, 0, -2)
	recent, err := repo.GetAll(ctx, domain.ExpenseFilter{FromDate: &from})
	if err != nil {
		t.Fatalf("GetAll by date failed: %v", err)
	}
	if len(recent) != 2 {
		t.Errorf("expected 2 recent expenses, got %d", len(recent))
	}
}