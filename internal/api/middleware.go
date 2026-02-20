package api

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
	"xkeen-panel/internal/auth"
)

type contextKey string

const usernameKey contextKey = "username"

// AuthMiddleware проверяет JWT-токен в заголовке Authorization
func AuthMiddleware(userManager *auth.UserManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")

			// Fallback на query param token (EventSource не поддерживает заголовки)
			if header == "" {
				if t := r.URL.Query().Get("token"); t != "" {
					header = "Bearer " + t
				}
			}

			if header == "" {
				http.Error(w, `{"error":"отсутствует токен авторизации"}`, http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, `{"error":"неверный формат токена"}`, http.StatusUnauthorized)
				return
			}

			user := userManager.GetUser()
			if user == nil {
				http.Error(w, `{"error":"пользователь не настроен"}`, http.StatusUnauthorized)
				return
			}

			username, err := auth.ValidateToken(parts[1], user.JWTSecret)
			if err != nil {
				http.Error(w, `{"error":"невалидный или просроченный токен"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), usernameKey, username)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RateLimiter — простой rate limiter для login
type RateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	maxAttempts int
	window      time.Duration
}

func NewRateLimiter(maxAttempts int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		attempts:    make(map[string][]time.Time),
		maxAttempts: maxAttempts,
		window:      window,
	}
}

// Allow проверяет, разрешён ли запрос с данного IP
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Очистить старые попытки
	var recent []time.Time
	for _, t := range rl.attempts[ip] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	rl.attempts[ip] = recent

	if len(recent) >= rl.maxAttempts {
		return false
	}

	rl.attempts[ip] = append(rl.attempts[ip], now)
	return true
}

// Reset сбрасывает счётчик для IP (после успешного входа)
func (rl *RateLimiter) Reset(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.attempts, ip)
}

// RateLimitMiddleware применяет rate limiting
func RateLimitMiddleware(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
				ip = strings.Split(forwarded, ",")[0]
			}

			if !limiter.Allow(strings.TrimSpace(ip)) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"слишком много попыток, попробуйте позже"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
