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

func TestInvalidOTPCodeMatchesStatus(t *testing.T) {
	if Code(ErrInvalidOTP) != 1400 || StatusCode(ErrInvalidOTP) != 400 {
		t.Fatalf("invalid OTP code/status = %d/%d, want 1400/400", Code(ErrInvalidOTP), StatusCode(ErrInvalidOTP))
	}
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		value  int64
		uz, en string
	}{
		{value: 0, uz: "0", en: "0"},
		{value: 999, uz: "999", en: "999"},
		{value: 1000, uz: "1 000", en: "1,000"},
		{value: 7200, uz: "7 200", en: "7,200"},
		{value: 1_000_000_000, uz: "1 000 000 000", en: "1,000,000,000"},
		{value: -7200, uz: "-7 200", en: "-7,200"},
	}
	for _, tt := range tests {
		got := FormatNumber(tt.value)
		if got.UZ != tt.uz || got.RU != tt.uz || got.EN != tt.en {
			t.Fatalf("FormatNumber(%d) = %+v, want uz/ru %q and en %q", tt.value, got, tt.uz, tt.en)
		}
	}
}
