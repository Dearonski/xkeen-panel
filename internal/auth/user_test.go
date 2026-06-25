package auth

import (
	"strings"
	"testing"
)

func TestSetupFlow(t *testing.T) {
	dir := t.TempDir()
	um := NewUserManager(dir)
	if err := um.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !um.SetupRequired() {
		t.Fatal("ожидался SetupRequired до создания пользователя")
	}

	if err := um.CreatePendingUser("bob", "password123", "TOTPSECRET"); err != nil {
		t.Fatalf("CreatePendingUser: %v", err)
	}
	if !um.HasPendingSetup() {
		t.Error("ожидался HasPendingSetup")
	}
	if um.GetPendingTOTPSecret() != "TOTPSECRET" {
		t.Error("неверный pending TOTP secret")
	}
	if !um.SetupRequired() {
		t.Error("до подтверждения пользователь не должен считаться настроенным")
	}

	if err := um.ConfirmSetup(); err != nil {
		t.Fatalf("ConfirmSetup: %v", err)
	}
	if um.SetupRequired() {
		t.Error("после подтверждения SetupRequired должен быть false")
	}
}

func TestCheckPassword(t *testing.T) {
	um := newConfirmedUser(t)

	if !um.CheckPassword("bob", "password123") {
		t.Error("верный пароль не прошёл")
	}
	if um.CheckPassword("bob", "wrong") {
		t.Error("неверный пароль прошёл")
	}
	if um.CheckPassword("alice", "password123") {
		t.Error("неверный логин прошёл")
	}
}

func TestAccessKeyLifecycle(t *testing.T) {
	dir := t.TempDir()
	um := newConfirmedUserIn(t, dir)

	if um.HasAccessKey() {
		t.Fatal("ключа быть не должно сразу после setup")
	}

	key, err := um.GenerateAccessKey()
	if err != nil {
		t.Fatalf("GenerateAccessKey: %v", err)
	}
	if !strings.HasPrefix(key, "xk_") {
		t.Errorf("ключ должен начинаться с xk_, got %q", key)
	}
	if !um.HasAccessKey() {
		t.Error("после генерации HasAccessKey должен быть true")
	}
	if hint := um.AccessKeyHint(); hint != key[len(key)-4:] {
		t.Errorf("hint = %q, want %q", hint, key[len(key)-4:])
	}

	if !um.CheckAccessKey(key) {
		t.Error("верный ключ не прошёл")
	}
	if um.CheckAccessKey("xk_wrong") {
		t.Error("неверный ключ прошёл")
	}

	// Перезагрузка из файла сохраняет ключ
	um2 := NewUserManager(dir)
	if err := um2.Load(); err != nil {
		t.Fatalf("reload Load: %v", err)
	}
	if !um2.CheckAccessKey(key) {
		t.Error("ключ не сохранился после перезагрузки")
	}
	if !um2.CheckPassword("bob", "password123") {
		t.Error("пароль не сохранился после перезагрузки")
	}

	// Перевыпуск инвалидирует старый ключ
	newKey, _ := um2.GenerateAccessKey()
	if um2.CheckAccessKey(key) {
		t.Error("старый ключ должен перестать работать после перевыпуска")
	}
	if !um2.CheckAccessKey(newKey) {
		t.Error("новый ключ должен работать")
	}

	// Отзыв
	if err := um2.RevokeAccessKey(); err != nil {
		t.Fatalf("RevokeAccessKey: %v", err)
	}
	if um2.HasAccessKey() {
		t.Error("после отзыва ключа быть не должно")
	}
	if um2.CheckAccessKey(newKey) {
		t.Error("отозванный ключ не должен работать")
	}
}

func newConfirmedUser(t *testing.T) *UserManager {
	return newConfirmedUserIn(t, t.TempDir())
}

func newConfirmedUserIn(t *testing.T, dir string) *UserManager {
	t.Helper()
	um := NewUserManager(dir)
	if err := um.CreatePendingUser("bob", "password123", "TOTPSECRET"); err != nil {
		t.Fatalf("CreatePendingUser: %v", err)
	}
	if err := um.ConfirmSetup(); err != nil {
		t.Fatalf("ConfirmSetup: %v", err)
	}
	return um
}
