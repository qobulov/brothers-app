package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/qobulov/brothers-app/pkg/apperror"
)

func TestNormalizeUsername(t *testing.T) {
	valid := map[string]string{
		"proniumq":              "proniumq",
		"  Qobulov  ":           "qobulov",
		"ABROR_755":             "abror_755",
		"a_1_b":                 "a_1_b",
		strings.Repeat("x", 32): strings.Repeat("x", 32),
	}
	for input, want := range valid {
		got, err := NormalizeUsername(input)
		if err != nil || got != want {
			t.Fatalf("NormalizeUsername(%q) = %q, %v; want %q", input, got, err, want)
		}
	}

	invalid := map[string]string{
		"":                      "Username kiritilishi shart",
		"   ":                   "Username kiritilishi shart",
		"abcd":                  "Username 5 dan 32 belgigacha bo'lishi kerak",
		strings.Repeat("x", 33): "Username 5 dan 32 belgigacha bo'lishi kerak",
		"suite-owner":           "Username faqat a-z, 0-9 va _ belgilaridan iborat bo'lishi mumkin",
		"ali vali":              "Username faqat a-z, 0-9 va _ belgilaridan iborat bo'lishi mumkin",
		"абдулла":               "Username faqat a-z, 0-9 va _ belgilaridan iborat bo'lishi mumkin",
		"@qobulov":              "Username faqat a-z, 0-9 va _ belgilaridan iborat bo'lishi mumkin",
	}
	for input, want := range invalid {
		_, err := NormalizeUsername(input)
		if !errors.Is(err, apperror.ErrInvalidData) {
			t.Fatalf("NormalizeUsername(%q) error = %v, want invalid data", input, err)
		}
		if got := apperror.MessageForLanguage(err, "uz"); got != want {
			t.Fatalf("NormalizeUsername(%q) message = %q, want %q", input, got, want)
		}
	}
}
