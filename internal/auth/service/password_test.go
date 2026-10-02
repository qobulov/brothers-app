package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/qobulov/brothers-app/pkg/apperror"
)

func TestValidPassword(t *testing.T) {
	for _, valid := range []string{"NewPass123", "Aa345678", "Strong-Pass#9 with spaces", strings.Repeat("Aa1", 24)} {
		if err := validPassword(valid); err != nil {
			t.Fatalf("validPassword(%q) = %v, want nil", valid, err)
		}
	}

	invalid := map[string]string{
		"":                        "Parol kamida 8 belgidan iborat bo'lishi kerak",
		"Aa1":                     "Parol kamida 8 belgidan iborat bo'lishi kerak",
		strings.Repeat("Aa1", 25): "Parol 72 belgidan oshmasligi kerak",
		"newpass123":              "Parolda kamida bitta katta harf bo'lishi kerak",
		"NEWPASS123":              "Parolda kamida bitta kichik harf bo'lishi kerak",
		"NewPassword":             "Parolda kamida bitta raqam bo'lishi kerak",
	}
	for password, want := range invalid {
		err := validPassword(password)
		if !errors.Is(err, apperror.ErrInvalidData) {
			t.Fatalf("validPassword(%q) error = %v, want invalid data", password, err)
		}
		if got := apperror.MessageForLanguage(err, "uz"); got != want {
			t.Fatalf("validPassword(%q) message = %q, want %q", password, got, want)
		}
	}
}
