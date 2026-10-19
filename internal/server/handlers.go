package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"expense-bot/internal/domain"
	"expense-bot/internal/repository"
)

const (
	// defaultLimit — размер страницы по умолчанию.
	defaultLimit = 50
	// maxLimit — максимальный размер страницы (защита от выгрузки всей БД).
	maxLimit = 500
	// maxBodyBytes — максимальный размер тела запроса.
	maxBodyBytes = 1 << 20 // 1 MiB
	// maxCommentLen — максимальная длина комментария в символах.
	maxCommentLen = 500
	// maxAmount — верхняя граница суммы траты.
	maxAmount = 1e12
)

// Handlers содержит зависимости HTTP-обработчиков.
type Handlers struct {
	repo repository.ExpenseRepository
}

// NewHandlers создаёт Handlers поверх репозитория трат.
func NewHandlers(repo repository.ExpenseRepository) *Handlers {
	return &Handlers{repo: repo}
}

// expenseResponse — представление траты в ответах API.
type expenseResponse struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Amount    float64   `json:"amount"`
	Category  string    `json:"category"`
	SpentAt   time.Time `json:"spent_at"`
	Comment   string    `json:"comment"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// errorResponse — единый формат ошибки API.
type errorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Details string `json:"details,omitempty"`
}

// createExpenseRequest — тело POST /api/v1/expenses и PUT /api/v1/expenses/{id}.
// Указатели позволяют отличить отсутствующее поле от нулевого значения.
type createExpenseRequest struct {
	UserID   *int64   `json:"user_id"`
	Amount   *float64 `json:"amount"`
	Category *string  `json:"category"`
	SpentAt  *string  `json:"spent_at"`
	Comment  *string  `json:"comment"`
}

// patchExpenseRequest — тело PATCH /api/v1/expenses/{id}: все поля опциональны.
type patchExpenseRequest struct {
	UserID   *int64   `json:"user_id"`
	Amount   *float64 `json:"amount"`
	Category *string  `json:"category"`
	SpentAt  *string  `json:"spent_at"`
	Comment  *string  `json:"comment"`
}

func toExpenseResponse(e *domain.Expense) expenseResponse {
	return expenseResponse{
		ID:        e.ID,
		UserID:    e.UserID,
		Amount:    e.Amount,
		Category:  e.Category,
		SpentAt:   e.SpentAt,
		Comment:   e.Comment,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
}

// writeJSON отправляет JSON-ответ с указанным статусом.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("failed to encode JSON response: %v", err)
	}
}

// writeError отправляет JSON-ошибку. Поле code дублирует HTTP-статус.
func writeError(w http.ResponseWriter, status int, message string, details ...string) {
	resp := errorResponse{Error: message, Code: status}
	if len(details) > 0 {
		resp.Details = details[0]
	}
	writeJSON(w, status, resp)
}

// parseID разбирает {id} из пути. При ошибке сам пишет ответ 400.
func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := r.PathValue("id")
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid expense id", "id must be a positive integer")
		return 0, false
	}
	return id, true
}

// decodeJSON читает строгий JSON из тела запроса: ограничивает размер,
// запрещает неизвестные поля и лишние значения после объекта.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst interface{}) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		switch {
		case errors.As(err, &maxErr):
			return fmt.Errorf("request body must not exceed %d bytes", maxBodyBytes)
		case errors.Is(err, io.EOF):
			return errors.New("request body is empty")
		default:
			return fmt.Errorf("malformed JSON: %v", err)
		}
	}
	if dec.More() {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

// validateAmount проверяет, что сумма положительна, конечна и не абсурдно велика.
func validateAmount(amount float64) error {
	if math.IsNaN(amount) || math.IsInf(amount, 0) {
		return errors.New("amount must be a finite number")
	}
	if amount <= 0 {
		return errors.New("amount must be greater than 0")
	}
	if amount > maxAmount {
		return fmt.Errorf("amount must not exceed %.0f", float64(maxAmount))
	}
	return nil
}

// validateCategory проверяет категорию по белому списку и возвращает
// её канонический вид. Любая строка вне списка (в том числе попытки
// SQL-инъекций) отклоняется — в SQL она не попадает вовсе.
func validateCategory(category string) (string, error) {
	normalized := domain.NormalizeCategory(category)
	if !domain.IsValidCategory(normalized) {
		allowed := make([]string, 0, len(domain.AllowedCategories))
		for name := range domain.AllowedCategories {
			allowed = append(allowed, name)
		}
		return "", fmt.Errorf("category must be one of: %s", strings.Join(allowed, ", "))
	}
	return normalized, nil
}

// dateLayouts — поддерживаемые форматы даты/времени.
var dateLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// parseDateTime разбирает дату траты. Даты без времени трактуются как UTC.
func parseDateTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date %q, use RFC3339 or YYYY-MM-DD", value)
}

// isDateOnly сообщает, что строка содержит только дату без времени.
func isDateOnly(value string) bool {
	_, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	return err == nil
}

// parseFilterDate разбирает from/to. Для "to" с датой без времени берётся
// конец дня, иначе траты за этот день в выборку не попадут.
func parseFilterDate(name, value string, endOfDay bool) (time.Time, error) {
	t, err := parseDateTime(value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s: %v", name, err)
	}
	if endOfDay && isDateOnly(value) {
		t = t.Add(24*time.Hour - time.Nanosecond)
	}
	return t, nil
}

// sanitizeComment обрезает пробелы, ограничивает длину и запрещает
// управляющие символы (кроме перевода строки и табуляции).
func sanitizeComment(comment string) (string, error) {
	comment = strings.TrimSpace(comment)
	if utf8.RuneCountInString(comment) > maxCommentLen {
		return "", fmt.Errorf("comment must not exceed %d characters", maxCommentLen)
	}
	if strings.ContainsFunc(comment, func(r rune) bool {
		return r == 0 || (unicode.IsControl(r) && r != '\n' && r != '\t')
	}) {
		return "", errors.New("comment must not contain control characters")
	}
	return comment, nil
}

// toDomain валидирует тело запроса и строит доменную модель траты.
func (req *createExpenseRequest) toDomain() (*domain.Expense, error) {
	if req.UserID == nil {
		return nil, errors.New("user_id is required")
	}
	if *req.UserID <= 0 {
		return nil, errors.New("user_id must be a positive Telegram ID")
	}
	if req.Amount == nil {
		return nil, errors.New("amount is required")
	}
	if err := validateAmount(*req.Amount); err != nil {
		return nil, err
	}
	if req.Category == nil {
		return nil, errors.New("category is required")
	}
	category, err := validateCategory(*req.Category)
	if err != nil {
		return nil, err
	}
	if req.SpentAt == nil {
		return nil, errors.New("spent_at is required (RFC3339 or YYYY-MM-DD)")
	}
	spentAt, err := parseDateTime(*req.SpentAt)
	if err != nil {
		return nil, err
	}

	comment := ""
	if req.Comment != nil {
		comment, err = sanitizeComment(*req.Comment)
		if err != nil {
			return nil, err
		}
	}

	return &domain.Expense{
		UserID:   *req.UserID,
		Amount:   *req.Amount,
		Category: category,
		SpentAt:  spentAt,
		Comment:  comment,
	}, nil
}

// apply накладывает частичные изменения на существующую трату.
// Пустой набор полей считается ошибкой валидации.
func (req *patchExpenseRequest) apply(e *domain.Expense) error {
	if req.UserID == nil && req.Amount == nil && req.Category == nil &&
		req.SpentAt == nil && req.Comment == nil {
		return errors.New("at least one field must be provided")
	}

	if req.UserID != nil {
		if *req.UserID <= 0 {
			return errors.New("user_id must be a positive Telegram ID")
		}
		e.UserID = *req.UserID
	}
	if req.Amount != nil {
		if err := validateAmount(*req.Amount); err != nil {
			return err
		}
		e.Amount = *req.Amount
	}
	if req.Category != nil {
		category, err := validateCategory(*req.Category)
		if err != nil {
			return err
		}
		e.Category = category
	}
	if req.SpentAt != nil {
		spentAt, err := parseDateTime(*req.SpentAt)
		if err != nil {
			return err
		}
		e.SpentAt = spentAt
	}
	if req.Comment != nil {
		comment, err := sanitizeComment(*req.Comment)
		if err != nil {
			return err
		}
		e.Comment = comment
	}
	return nil
}

// parseListFilter разбирает query-параметры списка трат.
// Фильтры: user_id, category, from, to; пагинация: limit, offset.
func parseListFilter(r *http.Request) (domain.ExpenseFilter, error) {
	var filter domain.ExpenseFilter
	q := r.URL.Query()

	if v := strings.TrimSpace(q.Get("user_id")); v != "" {
		userID, err := strconv.ParseInt(v, 10, 64)
		if err != nil || userID <= 0 {
			return filter, errors.New("user_id must be a positive integer")
		}
		filter.UserID = &userID
	}

	if v := strings.TrimSpace(q.Get("category")); v != "" {
		category, err := validateCategory(v)
		if err != nil {
			return filter, err
		}
		filter.Category = &category
	}

	if v := strings.TrimSpace(q.Get("from")); v != "" {
		from, err := parseFilterDate("from", v, false)
		if err != nil {
			return filter, err
		}
		filter.FromDate = &from
	}

	if v := strings.TrimSpace(q.Get("to")); v != "" {
		to, err := parseFilterDate("to", v, true)
		if err != nil {
			return filter, err
		}
		filter.ToDate = &to
	}

	if filter.FromDate != nil && filter.ToDate != nil && filter.FromDate.After(*filter.ToDate) {
		return filter, errors.New("from must be earlier than or equal to to")
	}

	filter.Limit = defaultLimit
	if v := strings.TrimSpace(q.Get("limit")); v != "" {
		limit, err := strconv.Atoi(v)
		if err != nil || limit < 1 || limit > maxLimit {
			return filter, fmt.Errorf("limit must be an integer between 1 and %d", maxLimit)
		}
		filter.Limit = limit
	}

	if v := strings.TrimSpace(q.Get("offset")); v != "" {
		offset, err := strconv.Atoi(v)
		if err != nil || offset < 0 {
			return filter, errors.New("offset must be a non-negative integer")
		}
		filter.Offset = offset
	}

	return filter, nil
}

// Health обрабатывает GET /healthz.
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ListExpenses обрабатывает GET /api/v1/expenses.
func (h *Handlers) ListExpenses(w http.ResponseWriter, r *http.Request) {
	filter, err := parseListFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}

	expenses, err := h.repo.GetAll(r.Context(), filter)
	if err != nil {
		log.Printf("list expenses: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve expenses")
		return
	}

	resp := make([]expenseResponse, 0, len(expenses))
	for _, e := range expenses {
		resp = append(resp, toExpenseResponse(e))
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetExpense обрабатывает GET /api/v1/expenses/{id}.
func (h *Handlers) GetExpense(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	expense, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		log.Printf("get expense %d: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve expense")
		return
	}
	if expense == nil {
		writeError(w, http.StatusNotFound, "expense not found")
		return
	}

	writeJSON(w, http.StatusOK, toExpenseResponse(expense))
}

// CreateExpense обрабатывает POST /api/v1/expenses.
func (h *Handlers) CreateExpense(w http.ResponseWriter, r *http.Request) {
	var req createExpenseRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	expense, err := req.toDomain()
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation failed", err.Error())
		return
	}

	if err := h.repo.Create(r.Context(), expense); err != nil {
		log.Printf("create expense: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create expense")
		return
	}

	created, err := h.repo.GetByID(r.Context(), expense.ID)
	if err != nil || created == nil {
		log.Printf("read created expense %d: %v", expense.ID, err)
		writeError(w, http.StatusInternalServerError, "failed to read created expense")
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/api/v1/expenses/%d", created.ID))
	writeJSON(w, http.StatusCreated, toExpenseResponse(created))
}

// ReplaceExpense обрабатывает PUT /api/v1/expenses/{id}: полная замена ресурса.
func (h *Handlers) ReplaceExpense(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	var req createExpenseRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	expense, err := req.toDomain()
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation failed", err.Error())
		return
	}

	existing, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		log.Printf("get expense %d: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve expense")
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "expense not found")
		return
	}

	expense.ID = id
	if err := h.repo.Update(r.Context(), expense); err != nil {
		log.Printf("replace expense %d: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to update expense")
		return
	}

	h.writeUpdatedExpense(w, r, id)
}

// PatchExpense обрабатывает PATCH /api/v1/expenses/{id}: частичное обновление.
func (h *Handlers) PatchExpense(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	var req patchExpenseRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	existing, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		log.Printf("get expense %d: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve expense")
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "expense not found")
		return
	}

	if err := req.apply(existing); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation failed", err.Error())
		return
	}

	if err := h.repo.Update(r.Context(), existing); err != nil {
		log.Printf("patch expense %d: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to update expense")
		return
	}

	h.writeUpdatedExpense(w, r, id)
}

// DeleteExpense обрабатывает DELETE /api/v1/expenses/{id}.
func (h *Handlers) DeleteExpense(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	existing, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		log.Printf("get expense %d: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve expense")
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "expense not found")
		return
	}

	if err := h.repo.Delete(r.Context(), id); err != nil {
		log.Printf("delete expense %d: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to delete expense")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// writeUpdatedExpense перечитывает трату из БД и отдаёт её клиенту,
// чтобы в ответе были актуальные updated_at.
func (h *Handlers) writeUpdatedExpense(w http.ResponseWriter, r *http.Request, id int64) {
	updated, err := h.repo.GetByID(r.Context(), id)
	if err != nil || updated == nil {
		log.Printf("read updated expense %d: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to read updated expense")
		return
	}
	writeJSON(w, http.StatusOK, toExpenseResponse(updated))
}
