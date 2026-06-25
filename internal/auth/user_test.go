package auth

import (
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
