package helpers

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/qobulov/brothers-app/pkg/apperror"
)

// NormalizePhone converts common human-entered phone formats to E.164-like form.
func NormalizePhone(value string) (string, error) {
	value = strings.TrimSpace(value)
	var digits strings.Builder
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case r == '+' || r == ' ' || r == '-' || r == '(' || r == ')':
		default:
			return "", apperror.ErrInvalidFormat
		}
	}
	normalized := digits.String()
	if len(normalized) < 9 || len(normalized) > 15 {
		return "", apperror.ErrInvalidFormat
	}
	return "+" + normalized, nil
}

// HashSecret is used for opaque tokens that must never be stored in plaintext.
func HashSecret(value string) string {
	sum := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// HashHMAC provides keyed hashing for low-entropy values such as OTPs.
func HashHMAC(value, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func RandomToken(size int) (string, error) {
	if size < 1 {
		return "", fmt.Errorf("token size must be positive")
	}
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func GenerateOTP() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	value := (uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])) % 1000000
	return fmt.Sprintf("%06d", value), nil
}

func ValidOTP(value string) bool {
	if len(value) != 6 {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
