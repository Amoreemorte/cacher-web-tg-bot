package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"expense-bot/internal/repository"
)

const (
	// shutdownTimeout — время на корректное завершение активных запросов.
	shutdownTimeout = 10 * time.Second
)

// Options содержит настройки HTTP-сервера.
type Options struct {
	// RateLimitRPS — допустимое число запросов в секунду от одного клиента.
	// Значение <= 0 отключает rate limiting.
	RateLimitRPS float64
	// RateLimitBurst — размер burst-бакета (сколько запросов можно сделать залпом).
	RateLimitBurst int
	// AllowedOrigins — origin'ы, которым разрешены CORS-запросы.
	// Пустой список или "*" разрешает все origin'ы.
	AllowedOrigins []string
	// Debug включает подробное логирование.
	Debug bool
}

// Server — HTTP-сервер REST API веб-панели.
// Зависит только от интерфейса repository.ExpenseRepository,
// поэтому легко подменяется моком в тестах.
type Server struct {
	repo    repository.ExpenseRepository
	handler http.Handler
	opts    Options
}

// NewServer собирает маршруты и цепочку middleware.
//
// Маршруты (идиоматичный REST):
//
//	GET    /healthz                     — проверка живости
//	GET    /api/v1/expenses             — список трат с фильтрами и пагинацией
//	POST   /api/v1/expenses             — создание траты
//	GET    /api/v1/expenses/{id}        — одна трата
//	PUT    /api/v1/expenses/{id}        — полная замена траты
//	PATCH  /api/v1/expenses/{id}        — частичное обновление траты
//	DELETE /api/v1/expenses/{id}        — удаление траты
func NewServer(repo repository.ExpenseRepository, opts Options) *Server {
	h := NewHandlers(repo)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.Health)
	mux.HandleFunc("GET /api/v1/expenses", h.ListExpenses)
	mux.HandleFunc("POST /api/v1/expenses", h.CreateExpense)
	mux.HandleFunc("GET /api/v1/expenses/{id}", h.GetExpense)
	mux.HandleFunc("PUT /api/v1/expenses/{id}", h.ReplaceExpense)
	mux.HandleFunc("PATCH /api/v1/expenses/{id}", h.PatchExpense)
	mux.HandleFunc("DELETE /api/v1/expenses/{id}", h.DeleteExpense)

	var handler http.Handler = mux
	handler = JSONErrorMiddleware(handler)
	handler = RecoveryMiddleware(handler)
	handler = RateLimitMiddleware(opts.RateLimitRPS, opts.RateLimitBurst)(handler)
	handler = CORSMiddleware(opts.AllowedOrigins)(handler)
	handler = SecurityHeadersMiddleware(handler)

	return &Server{repo: repo, handler: handler, opts: opts}
}

// Handler возвращает готовый http.Handler (используется в тестах и в main).
func (s *Server) Handler() http.Handler {
	return s.handler
}

// Run запускает сервер на addr и корректно останавливает его при отмене ctx.
func (s *Server) Run(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("REST API listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		log.Printf("REST API shutting down")
		return srv.Shutdown(shutdownCtx)
	}
}
