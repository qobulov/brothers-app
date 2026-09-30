package apperror

import (
	"errors"
	"fmt"
	"testing"
)

func TestLocalizedErrorMessage(t *testing.T) {
	err := fmt.Errorf("creating order: %w", New(ErrInvalidData, Text{UZ: "Summa noto'g'ri", RU: "Неверная сумма", EN: "Invalid amount"}))

	if !errors.Is(err, ErrInvalidData) {
		t.Fatal("a localized error must still match its kind")
	}
	if StatusCode(err) != 400 || Slug(err) != Slug(ErrInvalidData) {
		t.Fatalf("status/slug = %d/%s, want the kind's", StatusCode(err), Slug(err))
	}
	if err.Error() != "creating order: invalid data: Invalid amount" {
		t.Fatalf("Error() = %q", err.Error())
	}
	for language, want := range map[string]string{"uz": "Summa noto'g'ri", "ru-RU": "Неверная сумма", "en": "Invalid amount", "de": "Invalid amount"} {
		if got := MessageForLanguage(err, language); got != want {
			t.Fatalf("MessageForLanguage(%q) = %q, want %q", language, got, want)
		}
	}
	if got := MessageForLanguage(ErrInvalidData, "uz"); got != "Ma'lumotlar noto'g'ri" {
		t.Fatalf("plain kind message = %q, want the generic message", got)
	}
}

func TestLocalizedErrorField(t *testing.T) {
	err := fmt.Errorf("validating: %w", NewField(ErrInvalidData, "amount_usd", Text{UZ: "a", RU: "b", EN: "c"}))
	if Field(err) != "amount_usd" {
		t.Fatalf("Field() = %q, want amount_usd", Field(err))
	}
	if Field(New(ErrInvalidData, Text{EN: "x"})) != "" || Field(ErrInvalidData) != "" {
		t.Fatal("errors without a field must report none")
	}
}

func TestInvalidOTPCodeMatchesStatus(t *testing.T) {
	if Code(ErrInvalidOTP) != 1400 || StatusCode(ErrInvalidOTP) != 400 {
		t.Fatalf("invalid OTP code/status = %d/%d, want 1400/400", Code(ErrInvalidOTP), StatusCode(ErrInvalidOTP))
	}
}
