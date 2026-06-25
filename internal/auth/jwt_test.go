package auth

import "testing"

func TestGenerateValidateToken(t *testing.T) {
	token, err := GenerateToken("bob", "secret-key")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	sub, err := ValidateToken(token, "secret-key")
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if sub != "bob" {
		t.Errorf("sub = %q, want bob", sub)
	}
}

func TestValidateTokenWrongSecret(t *testing.T) {
	token, _ := GenerateToken("bob", "secret-key")
	if _, err := ValidateToken(token, "other-secret"); err == nil {
		t.Error("ожидалась ошибка для неверного секрета")
	}
}

func TestValidateTokenGarbage(t *testing.T) {
	if _, err := ValidateToken("not-a-token", "secret"); err == nil {
		t.Error("ожидалась ошибка для мусора")
	}
}
