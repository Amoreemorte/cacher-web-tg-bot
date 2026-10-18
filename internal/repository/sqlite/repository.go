package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"expense-bot/internal/domain"
	"expense-bot/internal/repository"

	_ "modernc.org/sqlite"
)

// SQLiteRepository реализует ExpenseRepository поверх SQLite.
type SQLiteRepository struct {
	db *sql.DB
}

// NewSQLiteRepository открывает файл базы данных и создаёт схему, если её нет.
func NewSQLiteRepository(dataSourceName string) (*SQLiteRepository, error) {
	db, err := sql.Open("sqlite", dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	if err := createSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	log.Printf("SQLite repository initialized: %s", dataSourceName)
	return &SQLiteRepository{db: db}, nil
}

// Close закрывает соединение с базой данных.
func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}

func createSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS expenses (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		amount REAL NOT NULL,
		category TEXT NOT NULL,
		spent_at TEXT NOT NULL,
		comment TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_expenses_user_id ON expenses(user_id);
	CREATE INDEX IF NOT EXISTS idx_expenses_spent_at ON expenses(spent_at);
	CREATE INDEX IF NOT EXISTS idx_expenses_category ON expenses(category);
	`
	_, err := db.Exec(schema)
	return err
}

var _ repository.ExpenseRepository = (*SQLiteRepository)(nil)

// Create сохраняет новую трату и присваивает ей сгенерированный ID.
func (r *SQLiteRepository) Create(ctx context.Context, expense *domain.Expense) error {
	query := `
		INSERT INTO expenses (user_id, amount, category, spent_at, comment, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := r.db.ExecContext(ctx, query,
		expense.UserID,
		expense.Amount,
		expense.Category,
		expense.SpentAt.UTC().Format(time.RFC3339),
		expense.Comment,
		now,
		now,
	)
	if err != nil {
		return fmt.Errorf("insert expense: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	expense.ID = id
	return nil
}

// GetByID возвращает трату по её идентификатору.
func (r *SQLiteRepository) GetByID(ctx context.Context, id int64) (*domain.Expense, error) {
	query := `
		SELECT id, user_id, amount, category, spent_at, comment, created_at, updated_at
		FROM expenses WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, query, id)
	return scanExpense(row)
}

// GetByUserID возвращает все траты пользователя, отсортированные по дате траты (по убыванию).
func (r *SQLiteRepository) GetByUserID(ctx context.Context, userID int64) ([]*domain.Expense, error) {
	query := `
		SELECT id, user_id, amount, category, spent_at, comment, created_at, updated_at
		FROM expenses WHERE user_id = ?
		ORDER BY spent_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("query expenses by user: %w", err)
	}
	defer rows.Close()
	return scanExpenses(rows)
}
// GetAll возвращает траты с учётом фильтра (пользователь, категория, период, пагинация).
func (r *SQLiteRepository) GetAll(ctx context.Context, filter domain.ExpenseFilter) ([]*domain.Expense, error) {
	query := `
		SELECT id, user_id, amount, category, spent_at, comment, created_at, updated_at
		FROM expenses WHERE 1 = 1
	`
	var args []interface{}

	if filter.UserID != nil {
		query += ` AND user_id = ?`
		args = append(args, *filter.UserID)
	}
	if filter.Category != nil {
		query += ` AND category = ?`
		args = append(args, *filter.Category)
	}
	if filter.FromDate != nil {
		query += ` AND spent_at >= ?`
		args = append(args, filter.FromDate.UTC().Format(time.RFC3339))
	}
	if filter.ToDate != nil {
		query += ` AND spent_at <= ?`
		args = append(args, filter.ToDate.UTC().Format(time.RFC3339))
	}

	query += ` ORDER BY spent_at DESC`

	if filter.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += ` OFFSET ?`
		args = append(args, filter.Offset)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query all expenses: %w", err)
	}
	defer rows.Close()
	return scanExpenses(rows)
}

// Update обновляет существующую трату по её ID.
func (r *SQLiteRepository) Update(ctx context.Context, expense *domain.Expense) error {
	query := `
		UPDATE expenses
		SET amount = ?, category = ?, spent_at = ?, comment = ?, updated_at = ?
		WHERE id = ?
	`
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx, query,
		expense.Amount,
		expense.Category,
		expense.SpentAt.UTC().Format(time.RFC3339),
		expense.Comment,
		now,
		expense.ID,
	)
	if err != nil {
		return fmt.Errorf("update expense: %w", err)
	}
	return nil
}

// Delete удаляет трату по её идентификатору.
func (r *SQLiteRepository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM expenses WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete expense: %w", err)
	}
	return nil
}

func scanExpense(row *sql.Row) (*domain.Expense, error) {
	return scanRow(row)
}

func scanExpenses(rows *sql.Rows) ([]*domain.Expense, error) {
	var expenses []*domain.Expense
	for rows.Next() {
		e, err := scanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan expense row: %w", err)
		}
		expenses = append(expenses, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate expenses: %w", err)
	}
	return expenses, nil
}

func scanRow(scanner interface {
	Scan(...interface{}) error
}) (*domain.Expense, error) {
	var e domain.Expense
	var spentAt, createdAt, updatedAt string
	var comment sql.NullString

	err := scanner.Scan(&e.ID, &e.UserID, &e.Amount, &e.Category, &spentAt, &comment, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}

	e.SpentAt, err = time.Parse(time.RFC3339, spentAt)
	if err != nil {
		return nil, fmt.Errorf("parse spent_at: %w", err)
	}
	e.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	e.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	if comment.Valid {
		e.Comment = comment.String
	}

	return &e, nil
}