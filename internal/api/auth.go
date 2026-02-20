package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"xkeen-panel/internal/auth"
	"xkeen-panel/internal/models"
)

type AuthHandler struct {
	userManager *auth.UserManager
	rateLimiter *RateLimiter
}

func NewAuthHandler(um *auth.UserManager, rl *RateLimiter) *AuthHandler {
	return &AuthHandler{userManager: um, rateLimiter: rl}
}

// HandleAuthStatus — GET /api/auth/status
func (h *AuthHandler) HandleAuthStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{
		"setup_required": h.userManager.SetupRequired(),
	})
}

// HandleSetup — POST /api/auth/setup
func (h *AuthHandler) HandleSetup(w http.ResponseWriter, r *http.Request) {
	if !h.userManager.SetupRequired() {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "пользователь уже настроен",
		})
		return
	}

	var req models.SetupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверный формат запроса"})
		return
	}

	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "логин и пароль обязательны"})
		return
	}

	if len(req.Password) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "пароль должен быть не менее 8 символов"})
		return
	}

	// Генерация TOTP
	secret, qrBase64, err := auth.GenerateTOTP(req.Username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ошибка генерации TOTP"})
		return
	}

	// Создание pending-пользователя
	if err := h.userManager.CreatePendingUser(req.Username, req.Password, secret); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ошибка создания пользователя"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"totp_secret": secret,
		"totp_qr":     qrBase64,
	})
}

// HandleSetupConfirm — POST /api/auth/setup/confirm
func (h *AuthHandler) HandleSetupConfirm(w http.ResponseWriter, r *http.Request) {
	if !h.userManager.HasPendingSetup() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "setup не начат"})
		return
	}

	var req models.SetupConfirmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверный формат запроса"})
		return
	}

	secret := h.userManager.GetPendingTOTPSecret()
	if !auth.ValidateTOTP(req.Code, secret) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверный TOTP-код"})
		return
	}

	if err := h.userManager.ConfirmSetup(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ошибка сохранения пользователя"})
		return
	}

	user := h.userManager.GetUser()
	token, err := auth.GenerateToken(user.Username, user.JWTSecret)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ошибка генерации токена"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

// HandleLogin — POST /api/auth/login
func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	ip := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ip = strings.Split(forwarded, ",")[0]
	}

	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверный формат запроса"})
		return
	}

	if !h.userManager.CheckPassword(req.Username, req.Password) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "неверный логин или пароль"})
		return
	}

	user := h.userManager.GetUser()
	if !auth.ValidateTOTP(req.TOTPCode, user.TOTPSecret) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "неверный TOTP-код"})
		return
	}

	token, err := auth.GenerateToken(user.Username, user.JWTSecret)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ошибка генерации токена"})
		return
	}

	// Сброс rate limiter после успешного входа
	h.rateLimiter.Reset(strings.TrimSpace(ip))

	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
