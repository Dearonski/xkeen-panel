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

// AuthMiddleware validates the JWT from the Authorization header.
func AuthMiddleware(userManager *auth.UserManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")

			// Fall back to the token query param: EventSource cannot send headers
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

// RateLimiter is a simple per-IP limiter for login attempts.
type RateLimiter struct {
	mu          sync.Mutex
	attempts    map[string][]time.Time
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

// Allow reports whether a request from this IP may proceed.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Drop attempts outside the window
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

// Reset clears the counter for an IP after a successful login.
func (rl *RateLimiter) Reset(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.attempts, ip)
}

// RateLimitMiddleware applies the rate limiter to a route.
func RateLimitMiddleware(limiter *RateLimiter, trustProxy bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow(clientIP(r, trustProxy)) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"слишком много попыток, попробуйте позже"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
