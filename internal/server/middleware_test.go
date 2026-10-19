package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRateLimitMiddleware_BurstExceeded(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := RateLimitMiddleware(0.001, 1)(ok)

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/api/v1/expenses", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want %d", first.Code, http.StatusOK)
	}

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/api/v1/expenses", nil))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want %d", second.Code, http.StatusTooManyRequests)
	}
	if got := second.Header().Get("Retry-After"); got != "1" {
		t.Errorf("Retry-After = %q, want 1", got)
	}

	var resp errorResponse
	if err := json.Unmarshal(second.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode 429 body %q: %v", second.Body.String(), err)
	}
	if resp.Code != http.StatusTooManyRequests || resp.Error == "" {
		t.Errorf("unexpected 429 payload: %+v", resp)
	}
}

func TestRateLimitMiddleware_Burst(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := RateLimitMiddleware(0.001, 3)(ok)

	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, want %d", i+1, rec.Code, http.StatusOK)
		}
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("request 4 status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
}

func TestRateLimitMiddleware_PerClient(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := RateLimitMiddleware(0.001, 1)(ok)

	send := func(ip string) int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Forwarded-For", ip+", 10.0.0.1")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := send("203.0.113.1"); code != http.StatusOK {
		t.Fatalf("client A first request status = %d", code)
	}
	if code := send("203.0.113.1"); code != http.StatusTooManyRequests {
		t.Fatalf("client A second request status = %d, want 429", code)
	}
	// Другой клиент имеет собственный бакет и не страдает от соседа.
	if code := send("203.0.113.2"); code != http.StatusOK {
		t.Fatalf("client B request status = %d, want %d", code, http.StatusOK)
	}
}

func TestRateLimitMiddleware_Disabled(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := RateLimitMiddleware(0, 0)(ok)

	for i := 0; i < 20; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, want %d", i+1, rec.Code, http.StatusOK)
		}
	}
}

func TestRateLimitAppliedByServer(t *testing.T) {
	srv := NewServer(nil, Options{RateLimitRPS: 0.001, RateLimitBurst: 1})

	first := doRequest(t, srv, http.MethodGet, "/healthz", nil)
	if first.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want %d", first.Code, http.StatusOK)
	}
	second := doRequest(t, srv, http.MethodGet, "/healthz", nil)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want %d", second.Code, http.StatusTooManyRequests)
	}
}

func TestCORSMiddleware_Preflight(t *testing.T) {
	srv, _ := newTestAPI(t, Options{AllowedOrigins: []string{"*"}})

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/expenses", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Allow-Origin = %q, want *", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "PATCH") {
		t.Errorf("Allow-Methods = %q, want PATCH included", got)
	}
}

func TestCORSMiddleware_AllowedOriginList(t *testing.T) {
	srv, _ := newTestAPI(t, Options{AllowedOrigins: []string{"http://localhost:5173"}})

	send := func(origin string) string {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/expenses", nil)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec.Header().Get("Access-Control-Allow-Origin")
	}

	if got := send("http://localhost:5173"); got != "http://localhost:5173" {
		t.Errorf("allowed origin header = %q, want http://localhost:5173", got)
	}
	if got := send("http://evil.example"); got != "" {
		t.Errorf("foreign origin header = %q, want empty", got)
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	panicking := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})
	handler := RecoveryMiddleware(panicking)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	var resp errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if resp.Code != http.StatusInternalServerError {
		t.Errorf("payload code = %d, want %d", resp.Code, http.StatusInternalServerError)
	}
}

func TestSecurityHeaders(t *testing.T) {
	srv, _ := newTestAPI(t, Options{})

	rec := doRequest(t, srv, http.MethodGet, "/healthz", nil)
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q, want DENY", got)
	}
}
