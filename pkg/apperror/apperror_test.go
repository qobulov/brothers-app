package apperror

import (
	"testing"
)

func TestRegistrationIdentityExistsMessage(t *testing.T) {
	tests := []struct {
		name     string
		language string
		want     string
	}{
		{name: "uzbek", language: "uz", want: "Email yoki foydalanuvchi nomi allaqachon mavjud"},
		{name: "russian", language: "ru", want: "Email или имя пользователя уже существуют"},
		{name: "english", language: "en", want: "Email or username already exists"},
		{name: "accept language priority", language: "de-DE,de;q=0.9,uz-UZ;q=0.8,en;q=0.7", want: "Email yoki foydalanuvchi nomi allaqachon mavjud"},
		{name: "highest accept language priority", language: "uz;q=0.1,ru;q=0.9,en;q=0.8", want: "Email или имя пользователя уже существуют"},
		{name: "unsupported defaults to english", language: "de", want: "Email or username already exists"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := MessageForLanguage(ErrRegistrationIdentityExists, test.language); got != test.want {
				t.Fatalf("MessageForLanguage() = %q, want %q", got, test.want)
			}
		})
	}

	if got := StatusCode(ErrRegistrationIdentityExists); got != 409 {
		t.Fatalf("StatusCode() = %d, want 409", got)
	}
	if got := Code(ErrRegistrationIdentityExists); got != 1409 {
		t.Fatalf("Code() = %d, want 1409", got)
	}
	if got := Slug(ErrRegistrationIdentityExists); got != "email_or_username_exists" {
		t.Fatalf("Slug() = %q, want %q", got, "email_or_username_exists")
	}
}
