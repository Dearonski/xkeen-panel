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
			t.Fatalf("попытка %d должна быть разрешена", i+1)
		}
	}
	if rl.Allow("1.1.1.1") {
		t.Fatal("4-я попытка должна быть отклонена")
	}

	// Другой IP считается независимо
	if !rl.Allow("2.2.2.2") {
		t.Fatal("первая попытка с другого IP должна быть разрешена")
	}

	rl.Reset("1.1.1.1")
	if !rl.Allow("1.1.1.1") {
		t.Fatal("после Reset попытка должна быть снова разрешена")
	}
}

func TestRateLimiterWindowExpiry(t *testing.T) {
	rl := NewRateLimiter(2, 50*time.Millisecond)

	if !rl.Allow("9.9.9.9") {
		t.Fatal("первая попытка должна быть разрешена")
	}
	if !rl.Allow("9.9.9.9") {
		t.Fatal("вторая попытка должна быть разрешена")
	}
	if rl.Allow("9.9.9.9") {
		t.Fatal("третья попытка должна быть отклонена")
	}

	time.Sleep(70 * time.Millisecond)

	if !rl.Allow("9.9.9.9") {
		t.Fatal("после истечения окна попытка должна быть разрешена")
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	limiter := NewRateLimiter(1, time.Minute)
	handler := RateLimitMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req1 := httptest.NewRequest(http.MethodGet, "/login", nil)
	req1.RemoteAddr = "5.5.5.5:12345"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("первый запрос: ожидался 200, получен %d", rec1.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/login", nil)
	req2.RemoteAddr = "5.5.5.5:12345"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("второй запрос: ожидался 429, получен %d", rec2.Code)
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
		t.Fatal("ожидался настроенный пользователь")
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
		{"без токена", "", "", http.StatusUnauthorized},
		{"валидный Bearer", "Bearer " + token, "", http.StatusOK},
		{"токен в query param", "", "?token=" + token, http.StatusOK},
		{"невалидный токен", "Bearer garbage", "", http.StatusUnauthorized},
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
				t.Fatalf("ожидался код %d, получен %d", tt.wantCode, rec.Code)
			}
		})
	}
}
