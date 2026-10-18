package domain

import "time"

// Expense представляет запись о семейной трате.
type Expense struct {
	ID        int64
	UserID    int64
	Amount    float64
	Category  string
	SpentAt   time.Time
	Comment   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AllowedCategories содержит допустимые категории трат.
var AllowedCategories = map[string]struct{}{
	"еда":            {},
	"транспорт":      {},
	"жильё":          {},
	"развлечения":    {},
	"здоровье":       {},
	"покупки":        {},
	"другое":         {},
}

// ExpenseFilter задаёт параметры фильтрации при получении списка трат.
type ExpenseFilter struct {
	UserID   *int64
	Category *string
	FromDate *time.Time
	ToDate   *time.Time
	Limit    int
	Offset   int
}