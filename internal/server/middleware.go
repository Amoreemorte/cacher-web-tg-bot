package server

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	// rateLimiterCleanupInterval — как часто удаляются простаивающие бакеты.
	rateLimiterCleanupInterval = time.Minute
	// rateLimiterIdleTTL — сколько живёт бакет неактивного клиента.
	rateLimiterIdleTTL = 10 * time.Minute
)

// clientBucket — токен-бакет одного клиента.
type clientBucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// rateLimiter хранит отдельный токен-бакет на каждого клиента.
// Бакеты простаивающих клиентов периодически удаляются, чтобы карта не росла
// бесконечно (защита от утечки памяти при большом числе IP).
type rateLimiter struct {
	mu          sync.Mutex
	buckets     map[string]*clientBucket
	rps         rate.Limit
	burst       int
	lastCleanup time.Time
}

func newRateLimiter(rps rate.Limit, burst int) *rateLimiter {
	return &rateLimiter{
		buckets:     make(map[string]*clientBucket),
		rps:         rps,
		burst:       burst,
		lastCleanup: time.Now(),
	}
}

// allow проверяет, есть ли у клиента свободный токен.
func (rl *rateLimiter) allow(key string) bool {
	now := time.Now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	if now.Sub(rl.lastCleanup) > rateLimiterCleanupInterval {
		for k, b := range rl.buckets {
			if now.Sub(b.lastSeen) > rateLimiterIdleTTL {
				delete(rl.buckets, k)
			}
		}
		rl.lastCleanup = now
	}

	bucket, ok := rl.buckets[key]
	if !ok {
		bucket = &clientBucket{limiter: rate.NewLimiter(rl.rps, rl.burst)}
		rl.buckets[key] = bucket
	}
	bucket.lastSeen = now

	return bucket.limiter.Allow()
}

// RateLimitMiddleware включает глобальный (для всех маршрутов API) rate limiting.
// Лимит применяется к каждому клиенту отдельно по токен-бакету: burst задаёт
// допустимый залп, rps — скорость восстановления токенов. При исчерпании
// бакета запрос отклоняется с HTTP 429 и заголовком Retry-After.
// Значение rps <= 0 отключает ограничение.
func RateLimitMiddleware(rps float64, burst int) func(http.Handler) http.Handler {
	if rps <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	if burst <= 0 {
		burst = 1
	}

	rl := newRateLimiter(rate.Limit(rps), burst)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !rl.allow(clientKey(r)) {
				w.Header().Set("Retry-After", "1")
				writeError(w, http.StatusTooManyRequests, "rate limit exceeded", "too many requests, try again later")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientKey определяет клиента для rate limiting.
// За прокси/балансировщиком берём первый адрес из X-Forwarded-For.
func clientKey(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
			return first
		}
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		return xri
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// CORSMiddleware разрешает запросы веб-панели с другого origin.
// Пустой список origins (или "*") разрешает все origin'ы.
// Preflight-запросы (OPTIONS) отвечаются до rate limiting и обработчиков.
func CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allowAll := len(allowedOrigins) == 0
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		origin = strings.TrimSpace(origin)
		switch {
		case origin == "*":
			allowAll = true
		case origin != "":
			allowed[origin] = struct{}{}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if allowAll {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else if _, ok := allowed[origin]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Add("Vary", "Origin")
				}
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
			w.Header().Set("Access-Control-Max-Age", "600")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RecoveryMiddleware превращает панику в обработчике в JSON-ответ 500,
// чтобы одно упавшее соединение не роняло весь сервер.
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic while handling %s %s: %v\n%s", r.Method, r.URL.Path, rec, debug.Stack())
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// SecurityHeadersMiddleware добавляет базовые защитные заголовки.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// JSONErrorMiddleware превращает текстовые 404/405, которые генерирует
// http.ServeMux, в единый JSON-формат ошибок API.
func JSONErrorMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&jsonErrorWriter{ResponseWriter: w}, r)
	})
}

type jsonErrorWriter struct {
	http.ResponseWriter
	wroteHeader bool
	intercepted bool
}

func (w *jsonErrorWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true

	isTextError := code == http.StatusNotFound || code == http.StatusMethodNotAllowed
	if isTextError && strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain") {
		w.intercepted = true
		message := "resource not found"
		if code == http.StatusMethodNotAllowed {
			message = "method not allowed"
		}
		body, err := json.Marshal(errorResponse{Error: message, Code: code})
		if err != nil {
			w.ResponseWriter.WriteHeader(code)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.ResponseWriter.WriteHeader(code)
		_, _ = w.ResponseWriter.Write(body)
		return
	}

	w.ResponseWriter.WriteHeader(code)
}

func (w *jsonErrorWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.intercepted {
		// Тело текстовой ошибки уже заменено JSON-ом.
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}
