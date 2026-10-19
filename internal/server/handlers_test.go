package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"expense-bot/internal/domain"
	"expense-bot/internal/repository"
	"expense-bot/internal/repository/sqlite"
)

// newTestAPI создаёт сервер поверх временной SQLite БД.
// Rate limiting по умолчанию выключен: для него есть отдельные тесты.
func newTestAPI(t *testing.T, opts Options) (*Server, repository.ExpenseRepository) {
	t.Helper()

	repo, err := sqlite.NewSQLiteRepository(filepath.Join(t.TempDir(), "test_server.db"))
	if err != nil {
		t.Fatalf("create test repository: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	opts.RateLimitRPS = 0
	return NewServer(repo, opts), repo
}

// doRequest выполняет запрос через полную цепочку middleware сервера.
// body может быть nil, строкой (сырой JSON) или любым значением для marshal.
func doRequest(t *testing.T, srv *Server, method, target string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	switch v := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(v)
	default:
		buf, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		reader = bytes.NewReader(buf)
	}

	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func decodeExpense(t *testing.T, rec *httptest.ResponseRecorder) expenseResponse {
	t.Helper()
	var resp expenseResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode expense response %q: %v", rec.Body.String(), err)
	}
	return resp
}

func decodeExpenses(t *testing.T, rec *httptest.ResponseRecorder) []expenseResponse {
	t.Helper()
	var resp []expenseResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode expenses response %q: %v", rec.Body.String(), err)
	}
	return resp
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) errorResponse {
	t.Helper()
	var resp errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error response %q: %v", rec.Body.String(), err)
	}
	return resp
}

func seedExpense(t *testing.T, repo repository.ExpenseRepository, e domain.Expense) *domain.Expense {
	t.Helper()
	if e.SpentAt.IsZero() {
		e.SpentAt = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	}
	if err := repo.Create(context.Background(), &e); err != nil {
		t.Fatalf("seed expense: %v", err)
	}
	return &e
}

func TestHealth(t *testing.T) {
	srv, _ := newTestAPI(t, Options{})

	rec := doRequest(t, srv, http.MethodGet, "/healthz", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
}

func TestListExpenses_EmptyReturnsJSONArray(t *testing.T) {
	srv, _ := newTestAPI(t, Options{})

	rec := doRequest(t, srv, http.MethodGet, "/api/v1/expenses", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
		t.Errorf("body = %q, want []", body)
	}
}

func TestListExpenses_FiltersAndPagination(t *testing.T) {
	srv, repo := newTestAPI(t, Options{})

	seedExpense(t, repo, domain.Expense{UserID: 1, Amount: 100, Category: "еда", SpentAt: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC), Comment: "завтрак"})
	seedExpense(t, repo, domain.Expense{UserID: 1, Amount: 200, Category: "транспорт", SpentAt: time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC), Comment: "такси"})
	seedExpense(t, repo, domain.Expense{UserID: 2, Amount: 300, Category: "еда", SpentAt: time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC), Comment: "обед"})

	tests := []struct {
		name  string
		query string
		want  int
	}{
		{name: "all", query: "", want: 3},
		{name: "by user", query: "user_id=1", want: 2},
		{name: "by category", query: "category=" + url.QueryEscape("еда"), want: 2},
		{name: "by user and category", query: "user_id=1&category=" + url.QueryEscape("еда"), want: 1},
		{name: "by period", query: "from=2026-10-02&to=2026-10-02", want: 1},
		{name: "pagination limit", query: "limit=2", want: 2},
		{name: "pagination offset", query: "limit=2&offset=2", want: 1},
		{name: "empty result", query: "user_id=999", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := "/api/v1/expenses"
			if tt.query != "" {
				target += "?" + tt.query
			}
			rec := doRequest(t, srv, http.MethodGet, target, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
			}
			if got := len(decodeExpenses(t, rec)); got != tt.want {
				t.Errorf("expenses count = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestListExpenses_SortedBySpentAtDesc(t *testing.T) {
	srv, repo := newTestAPI(t, Options{})

	older := seedExpense(t, repo, domain.Expense{UserID: 1, Amount: 100, Category: "еда", SpentAt: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)})
	newer := seedExpense(t, repo, domain.Expense{UserID: 1, Amount: 200, Category: "еда", SpentAt: time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)})

	rec := doRequest(t, srv, http.MethodGet, "/api/v1/expenses", nil)
	got := decodeExpenses(t, rec)
	if len(got) != 2 {
		t.Fatalf("expenses count = %d, want 2", len(got))
	}
	if got[0].ID != newer.ID || got[1].ID != older.ID {
		t.Errorf("order = [%d %d], want [%d %d]", got[0].ID, got[1].ID, newer.ID, older.ID)
	}
}

func TestListExpenses_InvalidQuery(t *testing.T) {
	srv, _ := newTestAPI(t, Options{})

	tests := []struct {
		name  string
		query string
	}{
		{name: "user_id not a number", query: "user_id=abc"},
		{name: "user_id negative", query: "user_id=-5"},
		{name: "unknown category", query: "category=" + url.QueryEscape("хакер")},
		{name: "invalid from", query: "from=2026-13-45"},
		{name: "invalid to", query: "to=yesterday"},
		{name: "from after to", query: "from=2026-10-10&to=2026-10-01"},
		{name: "limit zero", query: "limit=0"},
		{name: "limit too big", query: "limit=501"},
		{name: "limit not a number", query: "limit=ten"},
		{name: "offset negative", query: "offset=-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doRequest(t, srv, http.MethodGet, "/api/v1/expenses?"+tt.query, nil)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			if resp := decodeError(t, rec); resp.Code != http.StatusBadRequest || resp.Error == "" {
				t.Errorf("unexpected error payload: %+v", resp)
			}
		})
	}
}

func TestListExpenses_SQLInjectionAttemptRejected(t *testing.T) {
	srv, repo := newTestAPI(t, Options{})
	seedExpense(t, repo, domain.Expense{UserID: 1, Amount: 100, Category: "еда"})

	// Категория вне белого списка: в SQL не попадает, запрос отклоняется.
	injection := url.QueryEscape("' OR 1=1 --")
	rec := doRequest(t, srv, http.MethodGet, "/api/v1/expenses?category="+injection, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	// user_id приводится к числу, поэтому инъекция невозможна.
	rec = doRequest(t, srv, http.MethodGet, "/api/v1/expenses?user_id="+url.QueryEscape("1 OR 1=1"), nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	// Таблица не пострадала и фильтр по пользователю по-прежнему работает.
	rec = doRequest(t, srv, http.MethodGet, "/api/v1/expenses?user_id=1", nil)
	if got := len(decodeExpenses(t, rec)); got != 1 {
		t.Errorf("expenses count = %d, want 1", got)
	}
}

func TestCreateExpense_Success(t *testing.T) {
	srv, repo := newTestAPI(t, Options{})

	body := map[string]interface{}{
		"user_id":  12345,
		"amount":   350.5,
		"category": "  ЕДА ",
		"spent_at": "2026-10-18",
		"comment":  "  кофе  ",
	}
	rec := doRequest(t, srv, http.MethodPost, "/api/v1/expenses", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	resp := decodeExpense(t, rec)
	if resp.ID <= 0 {
		t.Errorf("id = %d, want positive", resp.ID)
	}
	if resp.UserID != 12345 || resp.Amount != 350.5 {
		t.Errorf("user_id/amount = %d/%v, want 12345/350.5", resp.UserID, resp.Amount)
	}
	if resp.Category != "еда" {
		t.Errorf("category = %q, want еда (normalized)", resp.Category)
	}
	if resp.Comment != "кофе" {
		t.Errorf("comment = %q, want кофе (trimmed)", resp.Comment)
	}
	if want := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC); !resp.SpentAt.Equal(want) {
		t.Errorf("spent_at = %v, want %v", resp.SpentAt, want)
	}
	if loc := rec.Header().Get("Location"); loc != "/api/v1/expenses/"+strconv.FormatInt(resp.ID, 10) {
		t.Errorf("Location = %q, want /api/v1/expenses/%d", loc, resp.ID)
	}

	stored, err := repo.GetByID(context.Background(), resp.ID)
	if err != nil || stored == nil {
		t.Fatalf("expense not persisted: %v", err)
	}
	if stored.Amount != 350.5 || stored.Category != "еда" {
		t.Errorf("stored expense = %+v", stored)
	}
}

func TestCreateExpense_Validation(t *testing.T) {
	srv, _ := newTestAPI(t, Options{})

	valid := `"user_id":1,"amount":100,"category":"еда","spent_at":"2026-10-18"`

	tests := []struct {
		name string
		body string
		want int
	}{
		{name: "empty body", body: "", want: http.StatusBadRequest},
		{name: "malformed json", body: `{"user_id":`, want: http.StatusBadRequest},
		{name: "unknown field", body: `{` + valid + `,"hacker":true}`, want: http.StatusBadRequest},
		{name: "amount as string", body: `{"user_id":1,"amount":"100","category":"еда","spent_at":"2026-10-18"}`, want: http.StatusBadRequest},
		{name: "multiple objects", body: `{` + valid + `}{` + valid + `}`, want: http.StatusBadRequest},
		{name: "missing user_id", body: `{"amount":100,"category":"еда","spent_at":"2026-10-18"}`, want: http.StatusUnprocessableEntity},
		{name: "zero user_id", body: `{"user_id":0,"amount":100,"category":"еда","spent_at":"2026-10-18"}`, want: http.StatusUnprocessableEntity},
		{name: "missing amount", body: `{"user_id":1,"category":"еда","spent_at":"2026-10-18"}`, want: http.StatusUnprocessableEntity},
		{name: "zero amount", body: `{"user_id":1,"amount":0,"category":"еда","spent_at":"2026-10-18"}`, want: http.StatusUnprocessableEntity},
		{name: "negative amount", body: `{"user_id":1,"amount":-10,"category":"еда","spent_at":"2026-10-18"}`, want: http.StatusUnprocessableEntity},
		{name: "missing category", body: `{"user_id":1,"amount":100,"spent_at":"2026-10-18"}`, want: http.StatusUnprocessableEntity},
		{name: "unknown category", body: `{"user_id":1,"amount":100,"category":"крипта","spent_at":"2026-10-18"}`, want: http.StatusUnprocessableEntity},
		{name: "missing spent_at", body: `{"user_id":1,"amount":100,"category":"еда"}`, want: http.StatusUnprocessableEntity},
		{name: "invalid spent_at", body: `{"user_id":1,"amount":100,"category":"еда","spent_at":"18.10.2026"}`, want: http.StatusUnprocessableEntity},
		{name: "comment too long", body: `{"user_id":1,"amount":100,"category":"еда","spent_at":"2026-10-18","comment":"` + strings.Repeat("я", 501) + `"}`, want: http.StatusUnprocessableEntity},
		{name: "comment control chars", body: `{"user_id":1,"amount":100,"category":"еда","spent_at":"2026-10-18","comment":"a\u0007b"}`, want: http.StatusUnprocessableEntity},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doRequest(t, srv, http.MethodPost, "/api/v1/expenses", tt.body)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d, body: %s", rec.Code, tt.want, rec.Body.String())
			}
			if resp := decodeError(t, rec); resp.Error == "" {
				t.Errorf("error payload is empty: %s", rec.Body.String())
			}
		})
	}
}

func TestCreateExpense_BodyTooLarge(t *testing.T) {
	srv, _ := newTestAPI(t, Options{})

	huge := `{"user_id":1,"amount":100,"category":"еда","spent_at":"2026-10-18","comment":"` +
		strings.Repeat("a", maxBodyBytes+1) + `"}`
	rec := doRequest(t, srv, http.MethodPost, "/api/v1/expenses", huge)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateExpense_SQLInjectionInCommentIsStoredAsText(t *testing.T) {
	srv, repo := newTestAPI(t, Options{})

	payload := `{"user_id":1,"amount":100,"category":"еда","spent_at":"2026-10-18","comment":"'; DROP TABLE expenses; --"}`
	rec := doRequest(t, srv, http.MethodPost, "/api/v1/expenses", payload)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	// Таблица цела, а вредоносная строка сохранена как обычный текст.
	expenses, err := repo.GetAll(context.Background(), domain.ExpenseFilter{})
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(expenses) != 1 {
		t.Fatalf("expenses count = %d, want 1", len(expenses))
	}
	if expenses[0].Comment != "'; DROP TABLE expenses; --" {
		t.Errorf("comment = %q, want payload stored verbatim", expenses[0].Comment)
	}
}

func TestGetExpense(t *testing.T) {
	srv, repo := newTestAPI(t, Options{})
	seeded := seedExpense(t, repo, domain.Expense{UserID: 7, Amount: 42, Category: "здоровье", Comment: "аптека"})

	t.Run("found", func(t *testing.T) {
		rec := doRequest(t, srv, http.MethodGet, "/api/v1/expenses/"+strconv.FormatInt(seeded.ID, 10), nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		resp := decodeExpense(t, rec)
		if resp.ID != seeded.ID || resp.UserID != 7 || resp.Comment != "аптека" {
			t.Errorf("unexpected expense: %+v", resp)
		}
	})

	t.Run("not found", func(t *testing.T) {
		rec := doRequest(t, srv, http.MethodGet, "/api/v1/expenses/9999", nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("invalid id", func(t *testing.T) {
		for _, id := range []string{"abc", "0", "-1", "1.5"} {
			rec := doRequest(t, srv, http.MethodGet, "/api/v1/expenses/"+id, nil)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("id %q: status = %d, want %d", id, rec.Code, http.StatusBadRequest)
			}
		}
	})
}

func TestReplaceExpense_PUT(t *testing.T) {
	srv, repo := newTestAPI(t, Options{})
	seeded := seedExpense(t, repo, domain.Expense{UserID: 1, Amount: 100, Category: "еда", Comment: "старый"})
	path := "/api/v1/expenses/" + strconv.FormatInt(seeded.ID, 10)

	body := map[string]interface{}{
		"user_id":  2,
		"amount":   999,
		"category": "покупки",
		"spent_at": "2026-09-30T10:15:00Z",
	}
	rec := doRequest(t, srv, http.MethodPut, path, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	resp := decodeExpense(t, rec)
	if resp.ID != seeded.ID {
		t.Errorf("id = %d, want %d", resp.ID, seeded.ID)
	}
	if resp.UserID != 2 || resp.Amount != 999 || resp.Category != "покупки" {
		t.Errorf("expense not replaced: %+v", resp)
	}
	// PUT — полная замена: отсутствующий comment очищается.
	if resp.Comment != "" {
		t.Errorf("comment = %q, want empty", resp.Comment)
	}
}

func TestReplaceExpense_Errors(t *testing.T) {
	srv, repo := newTestAPI(t, Options{})
	seeded := seedExpense(t, repo, domain.Expense{UserID: 1, Amount: 100, Category: "еда"})
	path := "/api/v1/expenses/" + strconv.FormatInt(seeded.ID, 10)

	t.Run("not found", func(t *testing.T) {
		rec := doRequest(t, srv, http.MethodPut, "/api/v1/expenses/424242", `{"user_id":1,"amount":10,"category":"еда","spent_at":"2026-10-18"}`)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("validation", func(t *testing.T) {
		rec := doRequest(t, srv, http.MethodPut, path, `{"user_id":1,"amount":0,"category":"еда","spent_at":"2026-10-18"}`)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
		}
	})

	t.Run("malformed body", func(t *testing.T) {
		rec := doRequest(t, srv, http.MethodPut, path, `not-json`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("invalid id", func(t *testing.T) {
		rec := doRequest(t, srv, http.MethodPut, "/api/v1/expenses/abc", `{"user_id":1,"amount":10,"category":"еда","spent_at":"2026-10-18"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})
}

func TestPatchExpense_PartialUpdate(t *testing.T) {
	srv, repo := newTestAPI(t, Options{})
	seeded := seedExpense(t, repo, domain.Expense{
		UserID: 1, Amount: 100, Category: "еда",
		SpentAt: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC), Comment: "кофе",
	})
	path := "/api/v1/expenses/" + strconv.FormatInt(seeded.ID, 10)

	rec := doRequest(t, srv, http.MethodPatch, path, `{"amount":450}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	resp := decodeExpense(t, rec)
	if resp.Amount != 450 {
		t.Errorf("amount = %v, want 450", resp.Amount)
	}
	if resp.Category != "еда" || resp.Comment != "кофе" || resp.UserID != 1 {
		t.Errorf("other fields changed: %+v", resp)
	}
	if !resp.SpentAt.Equal(seeded.SpentAt) {
		t.Errorf("spent_at = %v, want %v", resp.SpentAt, seeded.SpentAt)
	}
}

func TestPatchExpense_Errors(t *testing.T) {
	srv, repo := newTestAPI(t, Options{})
	seeded := seedExpense(t, repo, domain.Expense{UserID: 1, Amount: 100, Category: "еда"})
	path := "/api/v1/expenses/" + strconv.FormatInt(seeded.ID, 10)

	tests := []struct {
		name string
		path string
		body string
		want int
	}{
		{name: "empty object", path: path, body: `{}`, want: http.StatusUnprocessableEntity},
		{name: "invalid category", path: path, body: `{"category":"крипта"}`, want: http.StatusUnprocessableEntity},
		{name: "negative amount", path: path, body: `{"amount":-1}`, want: http.StatusUnprocessableEntity},
		{name: "invalid spent_at", path: path, body: `{"spent_at":"вчера"}`, want: http.StatusUnprocessableEntity},
		{name: "malformed json", path: path, body: `{`, want: http.StatusBadRequest},
		{name: "not found", path: "/api/v1/expenses/9999", body: `{"amount":10}`, want: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doRequest(t, srv, http.MethodPatch, tt.path, tt.body)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d, body: %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestDeleteExpense(t *testing.T) {
	srv, repo := newTestAPI(t, Options{})
	seeded := seedExpense(t, repo, domain.Expense{UserID: 1, Amount: 100, Category: "еда"})
	path := "/api/v1/expenses/" + strconv.FormatInt(seeded.ID, 10)

	rec := doRequest(t, srv, http.MethodDelete, path, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if body := rec.Body.String(); body != "" {
		t.Errorf("body = %q, want empty", body)
	}

	// Повторное удаление — 404: ресурса больше нет.
	rec = doRequest(t, srv, http.MethodDelete, path, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("second delete status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	if stored, err := repo.GetByID(context.Background(), seeded.ID); err != nil || stored != nil {
		t.Errorf("expense still exists: %+v, err: %v", stored, err)
	}
}

func TestDeleteExpense_InvalidID(t *testing.T) {
	srv, _ := newTestAPI(t, Options{})

	rec := doRequest(t, srv, http.MethodDelete, "/api/v1/expenses/abc", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	srv, _ := newTestAPI(t, Options{})

	rec := doRequest(t, srv, http.MethodDelete, "/api/v1/expenses", nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestUnknownRouteReturnsJSON(t *testing.T) {
	srv, _ := newTestAPI(t, Options{})

	rec := doRequest(t, srv, http.MethodGet, "/api/v1/unknown", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if resp := decodeError(t, rec); resp.Error == "" || resp.Code != http.StatusNotFound {
		t.Errorf("unexpected error payload: %+v", resp)
	}
}
