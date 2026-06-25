package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
	"xkeen-panel/internal/auth"
	"xkeen-panel/internal/models"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

const webAuthnSessionTTL = 5 * time.Minute

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

// webAuthn собирает экземпляр WebAuthn под текущий origin: из конфига, а если
// RPID не задан — из заголовков прокси (за HTTPS-туннелем это самый надёжный путь).
func (h *WebAuthnHandler) webAuthn(r *http.Request) (*webauthn.WebAuthn, error) {
	rpID := strings.TrimSpace(h.cfg.WebAuthnRPID)
	origins := h.cfg.WebAuthnOrigins

	if rpID == "" {
		host := r.Header.Get("X-Forwarded-Host")
		if host == "" {
			host = r.Host
		}
		scheme := r.Header.Get("X-Forwarded-Proto")
		if scheme == "" {
			scheme = "https"
		}
		if len(origins) == 0 && host != "" {
			origins = []string{scheme + "://" + host}
		}
		if i := strings.IndexByte(host, ':'); i >= 0 {
			host = host[:i]
		}
		rpID = host
	}

	name := h.cfg.WebAuthnRPName
	if name == "" {
		name = "XKeen Panel"
	}

	return webauthn.New(&webauthn.Config{
		RPID:          rpID,
		RPDisplayName: name,
		RPOrigins:     origins,
	})
}

func (h *WebAuthnHandler) putSession(key string, data *webauthn.SessionData) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for k, s := range h.sessions {
		if time.Since(s.at) > webAuthnSessionTTL {
			delete(h.sessions, k)
		}
	}
	h.sessions[key] = webAuthnSession{data: data, at: time.Now()}
}

func (h *WebAuthnHandler) takeSession(key string) *webauthn.SessionData {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.sessions[key]
	if !ok {
		return nil
	}
	delete(h.sessions, key)
	if time.Since(s.at) > webAuthnSessionTTL {
		return nil
	}
	return s.data
}

// HandleRegisterBegin — POST /api/account/passkey/register/begin (защищённый)
func (h *WebAuthnHandler) HandleRegisterBegin(w http.ResponseWriter, r *http.Request) {
	wa, err := h.webAuthn(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "webauthn не настроен: " + err.Error()})
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

	h.putSession("register:"+user.WebAuthnName(), session)
	writeJSON(w, http.StatusOK, options)
}

// HandleRegisterFinish — POST /api/account/passkey/register/finish (защищённый)
func (h *WebAuthnHandler) HandleRegisterFinish(w http.ResponseWriter, r *http.Request) {
	wa, err := h.webAuthn(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	user, err := h.userManager.WebAuthnUser()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ошибка пользователя"})
		return
	}

	session := h.takeSession("register:" + user.WebAuthnName())
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
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	user, err := h.userManager.WebAuthnUser()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "пользователь не настроен"})
		return
	}

	options, session, err := wa.BeginLogin(user)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	h.putSession("login:"+user.WebAuthnName(), session)
	writeJSON(w, http.StatusOK, options)
}

// HandleLoginFinish — POST /api/auth/login/passkey/finish (без JWT, rate-limited)
func (h *WebAuthnHandler) HandleLoginFinish(w http.ResponseWriter, r *http.Request) {
	wa, err := h.webAuthn(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	user, err := h.userManager.WebAuthnUser()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "пользователь не настроен"})
		return
	}

	session := h.takeSession("login:" + user.WebAuthnName())
	if session == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "сессия входа истекла, начните заново"})
		return
	}

	cred, err := wa.FinishLogin(user, *session, r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "passkey не прошёл проверку"})
		return
	}

	// Клонированный аутентификатор: счётчик подписей не вырос — это сигнал компрометации
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

	h.rateLimiter.Reset(strings.TrimSpace(clientIP(r)))
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
