package auth

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"

	"github.com/pquerna/otp/totp"
)

// GenerateTOTP создаёт новый TOTP-секрет и QR-код
func GenerateTOTP(username string) (secret string, qrBase64 string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "XKeen Panel",
		AccountName: username,
	})
	if err != nil {
		return "", "", fmt.Errorf("генерация TOTP: %w", err)
	}

	// Генерация QR-кода
	img, err := key.Image(256, 256)
	if err != nil {
		return "", "", fmt.Errorf("генерация QR-изображения: %w", err)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", "", fmt.Errorf("кодирование PNG: %w", err)
	}

	qrBase64 = "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())

	return key.Secret(), qrBase64, nil
}

// ValidateTOTP проверяет TOTP-код
func ValidateTOTP(code, secret string) bool {
	return totp.Validate(code, secret)
}
