package repository

import (
	"context"

	"expense-bot/internal/domain"
)

// ExpenseRepository определяет интерфейс доступа к данным о тратах.
// Реализацию можно заменить на другую СУБД или мок в тестах.
type ExpenseRepository interface {
	Create(ctx context.Context, expense *domain.Expense) error
	GetByID(ctx context.Context, id int64) (*domain.Expense, error)
	GetByUserID(ctx context.Context, userID int64) ([]*domain.Expense, error)
	GetAll(ctx context.Context, filter domain.ExpenseFilter) ([]*domain.Expense, error)
	Update(ctx context.Context, expense *domain.Expense) error
	Delete(ctx context.Context, id int64) error
}