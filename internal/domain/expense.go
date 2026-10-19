package domain

import (
	"strings"
	"time"
)

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
	"еда":         {},
	"транспорт":   {},
	"жильё":       {},
	"развлечения": {},
	"здоровье":    {},
	"покупки":     {},
	"другое":      {},
}

// NormalizeCategory приводит категорию к каноническому виду:
// обрезает пробелы и переводит в нижний регистр.
func NormalizeCategory(category string) string {
	return strings.ToLower(strings.TrimSpace(category))
}

// IsValidCategory сообщает, входит ли категория в список допустимых.
func IsValidCategory(category string) bool {
	_, ok := AllowedCategories[NormalizeCategory(category)]
	return ok
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
