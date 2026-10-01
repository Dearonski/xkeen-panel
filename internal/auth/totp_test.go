package auth

import (
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

func TestGenerateAndValidateTOTP(t *testing.T) {
	secret, qr, err := GenerateTOTP("bob")
	if err != nil {
		t.Fatalf("GenerateTOTP: %v", err)
	}
	if secret == "" {
		t.Fatal("empty secret")
	}
	if len(qr) == 0 {
		t.Error("empty QR")
	}

	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if !ValidateTOTP(code, secret) {
		t.Error("a valid code failed validation")
	}
}

func TestValidateTOTPWrongSecret(t *testing.T) {
	secret1, _, _ := GenerateTOTP("bob")
	secret2, _, _ := GenerateTOTP("bob")

	code2, _ := totp.GenerateCode(secret2, time.Now())
	if ValidateTOTP(code2, secret1) {
		t.Error("a code from another secret must not validate")
	}
}
