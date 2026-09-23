package apperror

import "testing"

func TestRegistrationIdentityExistsMessage(t *testing.T) {
	tests := []struct {
		name     string
		language string
		want     string
	}{
		{name: "uzbek", language: "uz", want: "Telefon raqami yoki foydalanuvchi nomi allaqachon mavjud"},
		{name: "russian", language: "ru", want: "Номер телефона или имя пользователя уже существует"},
		{name: "english", language: "en", want: "Phone or username already exists"},
		{name: "accept language", language: "de-DE,de;q=0.9,uz-UZ;q=0.8,en;q=0.7", want: "Telefon raqami yoki foydalanuvchi nomi allaqachon mavjud"},
		{name: "unsupported defaults to english", language: "de", want: "Phone or username already exists"},
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
	if got := Slug(ErrRegistrationIdentityExists); got != "phone_or_username_exists" {
		t.Fatalf("Slug() = %q, want %q", got, "phone_or_username_exists")
	}
}
