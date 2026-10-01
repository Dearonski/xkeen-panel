package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"xkeen-panel/internal/auth"
)

func TestRateLimiterAllow(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)

	for i := 0; i < 3; i++ {
		if !rl.Allow("1.1.1.1") {
			t.Fatalf("attempt %d must be allowed", i+1)
		}
	}
	if rl.Allow("1.1.1.1") {
		t.Fatal("the 4th attempt must be rejected")
	}

	// A different IP is counted independently
	if !rl.Allow("2.2.2.2") {
		t.Fatal("the first attempt from another IP must be allowed")
	}

	rl.Reset("1.1.1.1")
	if !rl.Allow("1.1.1.1") {
		t.Fatal("after Reset an attempt must be allowed again")
	}
}

func TestRateLimiterWindowExpiry(t *testing.T) {
	rl := NewRateLimiter(2, 50*time.Millisecond)

	if !rl.Allow("9.9.9.9") {
		t.Fatal("the first attempt must be allowed")
	}
	if !rl.Allow("9.9.9.9") {
		t.Fatal("the second attempt must be allowed")
	}
	if rl.Allow("9.9.9.9") {
		t.Fatal("the third attempt must be rejected")
	}

	time.Sleep(70 * time.Millisecond)

	if !rl.Allow("9.9.9.9") {
		t.Fatal("an attempt must be allowed once the window expires")
	}
}

func TestClientIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "9.9.9.9:111"
	req.Header.Set("X-Forwarded-For", "1.1.1.1, 2.2.2.2")

	// Proxy not trusted: RemoteAddr wins and a spoofed XFF is ignored
	if got := clientIP(req, false); got != "9.9.9.9:111" {
		t.Errorf("trustProxy=false: got %q, want RemoteAddr", got)
	}
	// Proxy trusted: the rightmost hop, the one the proxy appended
	if got := clientIP(req, true); got != "2.2.2.2" {
		t.Errorf("trustProxy=true: got %q, want rightmost XFF", got)
	}
	// Trusted but no XFF: RemoteAddr
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "9.9.9.9:111"
	if got := clientIP(req2, true); got != "9.9.9.9:111" {
		t.Errorf("trustProxy=true without XFF: got %q", got)
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	limiter := NewRateLimiter(1, time.Minute)
	handler := RateLimitMiddleware(limiter, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req1 := httptest.NewRequest(http.MethodGet, "/login", nil)
	req1.RemoteAddr = "5.5.5.5:12345"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first request: got %d, want 200", rec1.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/login", nil)
	req2.RemoteAddr = "5.5.5.5:12345"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: got %d, want 429", rec2.Code)
	}
}

func newConfirmedUserManager(t *testing.T) *auth.UserManager {
	t.Helper()
	um := auth.NewUserManager(t.TempDir())
	if err := um.CreatePendingUser("bob", "password123", "SECRET"); err != nil {
		t.Fatalf("CreatePendingUser: %v", err)
	}
	if err := um.ConfirmSetup(); err != nil {
		t.Fatalf("ConfirmSetup: %v", err)
	}
	return um
}

func TestAuthMiddleware(t *testing.T) {
	um := newConfirmedUserManager(t)
	user := um.GetUser()
	if user == nil {
		t.Fatal("want a configured user")
	}
	token, err := auth.GenerateToken(user.Username, user.JWTSecret)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	handler := AuthMiddleware(um)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name       string
		authHeader string
		query      string
		wantCode   int
	}{
		{"no token", "", "", http.StatusUnauthorized},
		{"valid Bearer", "Bearer " + token, "", http.StatusOK},
		{"token in query param", "", "?token=" + token, http.StatusOK},
		{"invalid token", "Bearer garbage", "", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/status"+tt.query, nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.wantCode {
				t.Fatalf("got code %d, want %d", rec.Code, tt.wantCode)
			}
		})
	}
}
