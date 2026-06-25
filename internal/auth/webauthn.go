package auth

import (
	"crypto/rand"
	"encoding/base64"
	"os"

	"github.com/go-webauthn/webauthn/webauthn"
)

// webAuthnUser реализует интерфейс webauthn.User поверх снимка данных пользователя.
type webAuthnUser struct {
	id          []byte
	name        string
	credentials []webauthn.Credential
}

func (u *webAuthnUser) WebAuthnID() []byte                         { return u.id }
func (u *webAuthnUser) WebAuthnName() string                       { return u.name }
func (u *webAuthnUser) WebAuthnDisplayName() string                { return u.name }
func (u *webAuthnUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

// WebAuthnUser возвращает пользователя для церемоний WebAuthn, при необходимости
// генерируя стабильный user handle.
func (um *UserManager) WebAuthnUser() (webauthn.User, error) {
	um.mu.Lock()
	defer um.mu.Unlock()

	if um.user == nil {
		return nil, os.ErrNotExist
	}

	if len(um.user.WebAuthnID) == 0 {
		id := make([]byte, 32)
		if _, err := rand.Read(id); err != nil {
			return nil, err
		}
		um.user.WebAuthnID = id
		if err := um.persistLocked(); err != nil {
			return nil, err
		}
	}

	creds := make([]webauthn.Credential, len(um.user.Credentials))
	copy(creds, um.user.Credentials)

	return &webAuthnUser{id: um.user.WebAuthnID, name: um.user.Username, credentials: creds}, nil
}

// AddWebAuthnCredential добавляет новый passkey.
func (um *UserManager) AddWebAuthnCredential(cred *webauthn.Credential) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	if um.user == nil {
		return os.ErrNotExist
	}
	um.user.Credentials = append(um.user.Credentials, *cred)
	return um.persistLocked()
}

// UpdateWebAuthnCredential обновляет существующий passkey (например, счётчик подписей).
func (um *UserManager) UpdateWebAuthnCredential(cred *webauthn.Credential) {
	um.mu.Lock()
	defer um.mu.Unlock()

	if um.user == nil {
		return
	}
	for i := range um.user.Credentials {
		if string(um.user.Credentials[i].ID) == string(cred.ID) {
			um.user.Credentials[i] = *cred
			_ = um.persistLocked()
			return
		}
	}
}

// RemoveWebAuthnCredential удаляет passkey по base64url его ID.
func (um *UserManager) RemoveWebAuthnCredential(idB64 string) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	if um.user == nil {
		return os.ErrNotExist
	}
	kept := um.user.Credentials[:0]
	for _, c := range um.user.Credentials {
		if base64.RawURLEncoding.EncodeToString(c.ID) != idB64 {
			kept = append(kept, c)
		}
	}
	um.user.Credentials = kept
	return um.persistLocked()
}

// HasWebAuthnCredentials сообщает, зарегистрирован ли хотя бы один passkey.
func (um *UserManager) HasWebAuthnCredentials() bool {
	um.mu.RLock()
	defer um.mu.RUnlock()
	return um.user != nil && len(um.user.Credentials) > 0
}

// WebAuthnCredentialIDs возвращает base64url-идентификаторы зарегистрированных passkey.
func (um *UserManager) WebAuthnCredentialIDs() []string {
	um.mu.RLock()
	defer um.mu.RUnlock()
	if um.user == nil {
		return nil
	}
	ids := make([]string, 0, len(um.user.Credentials))
	for _, c := range um.user.Credentials {
		ids = append(ids, base64.RawURLEncoding.EncodeToString(c.ID))
	}
	return ids
}
