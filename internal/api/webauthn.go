package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
	"xkeen-panel/internal/auth"
	"xkeen-panel/internal/models"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

const (
	webAuthnSessionTTL  = 5 * time.Minute
	maxWebAuthnSessions = 64
	registerCookie      = "xkwa_reg"
	loginCookie         = "xkwa_login"
	registerPath        = "/api/account/passkey"
	loginPath           = "/api/auth/login/passkey"
)

type webAuthnSession struct {
	data *webauthn.SessionData
	at   time.Time
}

type WebAuthnHandler struct {
	userManager *auth.UserManager
	rateLimiter *RateLimiter
	cfg         *models.Config

	mu       sync.Mutex
	sessions map[string]webAuthnSession
}

func NewWebAuthnHandler(um *auth.UserManager, rl *RateLimiter, cfg *models.Config) *WebAuthnHandler {
	return &WebAuthnHandler{
		userManager: um,
		rateLimiter: rl,
		cfg:         cfg,
		sessions:    make(map[string]webAuthnSession),
	}
}

// webAuthn собирает экземпляр WebAuthn. RPID/origins берутся из конфига; вывод из
// заголовков запроса допускается только при доверенном прокси (иначе серверный
// origin-пин был бы подконтролен клиенту на прямом :3000 сокете).
func (h *WebAuthnHandler) webAuthn(r *http.Request) (*webauthn.WebAuthn, error) {
	rpID := strings.TrimSpace(h.cfg.WebAuthnRPID)
	origins := h.cfg.WebAuthnOrigins
	name := h.cfg.WebAuthnRPName
	if name == "" {
		name = "XKeen Panel"
	}

	if rpID == "" {
		if !h.cfg.TrustProxyHeaders {
			return nil, fmt.Errorf("passkey не настроен: задайте webauthn_rp_id и webauthn_origins (или trust_proxy_headers за доверенным прокси)")
		}
		host := r.Header.Get("X-Forwarded-Host")
		if host == "" {
			host = r.Host
		}
		if host == "" {
			return nil, fmt.Errorf("не удалось определить host для passkey")
		}
		scheme := r.Header.Get("X-Forwarded-Proto")
		if scheme == "" {
			if r.TLS != nil {
				scheme = "https"
			} else {
				scheme = "http"
			}
		}
		if len(origins) == 0 {
			origins = []string{scheme + "://" + host}
		}
		if i := strings.IndexByte(host, ':'); i >= 0 {
			host = host[:i]
		}
		rpID = host
	}

	return webauthn.New(&webauthn.Config{
		RPID:          rpID,
		RPDisplayName: name,
		RPOrigins:     origins,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			UserVerification: protocol.VerificationRequired,
		},
	})
}

func genCeremonyID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// putSession сохраняет SessionData под случайным id, подметая просроченное и
// ограничивая размер карты (защита от спама begin).
func (h *WebAuthnHandler) putSession(id string, data *webauthn.SessionData) {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	for k, s := range h.sessions {
		if now.Sub(s.at) > webAuthnSessionTTL {
			delete(h.sessions, k)
		}
	}
	if len(h.sessions) >= maxWebAuthnSessions {
		var oldestK string
		var oldestT time.Time
		first := true
		for k, s := range h.sessions {
			if first || s.at.Before(oldestT) {
				oldestK, oldestT, first = k, s.at, false
			}
		}
		delete(h.sessions, oldestK)
	}
	h.sessions[id] = webAuthnSession{data: data, at: now}
}

func (h *WebAuthnHandler) takeSession(id string) *webauthn.SessionData {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.sessions[id]
	if !ok {
		return nil
	}
	delete(h.sessions, id)
	if time.Since(s.at) > webAuthnSessionTTL {
		return nil
	}
	return s.data
}

func (h *WebAuthnHandler) setCeremonyCookie(w http.ResponseWriter, r *http.Request, name, path, id string) {
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    id,
		Path:     path,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(webAuthnSessionTTL.Seconds()),
	})
}

func clearCeremonyCookie(w http.ResponseWriter, name, path string) {
	http.SetCookie(w, &http.Cookie{Name: name, Path: path, MaxAge: -1, HttpOnly: true})
}

func cookieValue(r *http.Request, name string) string {
	if c, err := r.Cookie(name); err == nil {
		return c.Value
	}
	return ""
}

// HandleRegisterBegin — POST /api/account/passkey/register/begin (защищённый)
func (h *WebAuthnHandler) HandleRegisterBegin(w http.ResponseWriter, r *http.Request) {
	// Поддерживается ровно один passkey — чтобы добавить новый, сначала удалить текущий
	if h.userManager.HasWebAuthnCredentials() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "passkey уже добавлен — сначала удалите текущий"})
		return
	}

	wa, err := h.webAuthn(r)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	user, err := h.userManager.WebAuthnUser()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ошибка пользователя"})
		return
	}

	exclude := make([]protocol.CredentialDescriptor, 0)
	for _, c := range user.WebAuthnCredentials() {
		exclude = append(exclude, c.Descriptor())
	}

	options, session, err := wa.BeginRegistration(user, webauthn.WithExclusions(exclude))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	id, err := genCeremonyID()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ошибка сессии"})
		return
	}
	h.putSession(id, session)
	h.setCeremonyCookie(w, r, registerCookie, registerPath, id)
	writeJSON(w, http.StatusOK, options)
}

// HandleRegisterFinish — POST /api/account/passkey/register/finish (защищённый)
func (h *WebAuthnHandler) HandleRegisterFinish(w http.ResponseWriter, r *http.Request) {
	wa, err := h.webAuthn(r)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	user, err := h.userManager.WebAuthnUser()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ошибка пользователя"})
		return
	}

	clearCeremonyCookie(w, registerCookie, registerPath)
	session := h.takeSession(cookieValue(r, registerCookie))
	if session == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "сессия регистрации истекла, начните заново"})
		return
	}

	cred, err := wa.FinishRegistration(user, *session, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "не удалось зарегистрировать passkey: " + err.Error()})
		return
	}

	if err := h.userManager.AddWebAuthnCredential(cred); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ошибка сохранения passkey"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// HandleLoginBegin — POST /api/auth/login/passkey/begin (без JWT, rate-limited)
func (h *WebAuthnHandler) HandleLoginBegin(w http.ResponseWriter, r *http.Request) {
	if !h.userManager.HasWebAuthnCredentials() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "passkey не настроен"})
		return
	}

	wa, err := h.webAuthn(r)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	user, err := h.userManager.WebAuthnUser()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "пользователь не настроен"})
		return
	}

	options, session, err := wa.BeginLogin(user, webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	id, err := genCeremonyID()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ошибка сессии"})
		return
	}
	h.putSession(id, session)
	h.setCeremonyCookie(w, r, loginCookie, loginPath, id)
	writeJSON(w, http.StatusOK, options)
}

// HandleLoginFinish — POST /api/auth/login/passkey/finish (без JWT, rate-limited)
func (h *WebAuthnHandler) HandleLoginFinish(w http.ResponseWriter, r *http.Request) {
	wa, err := h.webAuthn(r)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	user, err := h.userManager.WebAuthnUser()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "пользователь не настроен"})
		return
	}

	clearCeremonyCookie(w, loginCookie, loginPath)
	session := h.takeSession(cookieValue(r, loginCookie))
	if session == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "сессия входа истекла, начните заново"})
		return
	}

	cred, err := wa.FinishLogin(user, *session, r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "passkey не прошёл проверку"})
		return
	}

	// Клонированный аутентификатор: счётчик подписей не вырос — сигнал компрометации
	if cred.Authenticator.CloneWarning {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "passkey отклонён (clone warning)"})
		return
	}

	h.userManager.UpdateWebAuthnCredential(cred)

	u := h.userManager.GetUser()
	token, err := auth.GenerateToken(u.Username, u.JWTSecret)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ошибка генерации токена"})
		return
	}

	h.rateLimiter.Reset(clientIP(r, h.cfg.TrustProxyHeaders))
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

// HandlePasskeyList — GET /api/account/passkey (защищённый)
func (h *WebAuthnHandler) HandlePasskeyList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"passkeys": h.userManager.WebAuthnCredentialIDs(),
	})
}

// HandlePasskeyDelete — DELETE /api/account/passkey (защищённый)
func (h *WebAuthnHandler) HandlePasskeyDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id обязателен"})
		return
	}
	if err := h.userManager.RemoveWebAuthnCredential(req.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ошибка удаления"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}
