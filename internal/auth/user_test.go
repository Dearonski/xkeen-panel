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
		t.Fatal("want SetupRequired before the user is created")
	}

	if err := um.CreatePendingUser("bob", "password123", "TOTPSECRET"); err != nil {
		t.Fatalf("CreatePendingUser: %v", err)
	}
	if !um.HasPendingSetup() {
		t.Error("want HasPendingSetup")
	}
	if um.GetPendingTOTPSecret() != "TOTPSECRET" {
		t.Error("wrong pending TOTP secret")
	}
	if !um.SetupRequired() {
		t.Error("the user must not count as set up before confirmation")
	}

	if err := um.ConfirmSetup(); err != nil {
		t.Fatalf("ConfirmSetup: %v", err)
	}
	if um.SetupRequired() {
		t.Error("SetupRequired must be false after confirmation")
	}
}

func TestCheckPassword(t *testing.T) {
	um := newConfirmedUser(t)

	if !um.CheckPassword("bob", "password123") {
		t.Error("the correct password was rejected")
	}
	if um.CheckPassword("bob", "wrong") {
		t.Error("a wrong password was accepted")
	}
	if um.CheckPassword("alice", "password123") {
		t.Error("a wrong username was accepted")
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
