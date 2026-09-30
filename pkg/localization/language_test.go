package localization

import "testing"

func TestResolveAcceptLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header string
		want   string
	}{
		{name: "empty defaults to English", want: "en"},
		{name: "language region is supported", header: "uz-UZ", want: "uz"},
		{name: "highest quality wins", header: "uz;q=0.1, ru-RU;q=0.9, en;q=0.8", want: "ru"},
		{name: "equal quality keeps header order", header: "ru,uz", want: "ru"},
		{name: "zero quality is ignored", header: "uz;q=0,ru;q=0.5", want: "ru"},
		{name: "only zero quality defaults to English", header: "uz;q=0,ru;q=0", want: "en"},
		{name: "unsupported defaults to English", header: "de-DE,de;q=0.9", want: "en"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ResolveAcceptLanguage(tt.header); got != tt.want {
				t.Fatalf("ResolveAcceptLanguage(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

func TestRequestLanguage(t *testing.T) {
	tests := []struct {
		name                string
		applicationLanguage string
		acceptLanguage      string
		want                string
	}{
		{name: "application language wins", applicationLanguage: "uz", acceptLanguage: "ru", want: "uz"},
		{name: "application language is case-insensitive", applicationLanguage: " RU ", want: "ru"},
		{name: "application language with region", applicationLanguage: "uz-UZ", want: "uz"},
		{name: "unsupported application language falls back", applicationLanguage: "de", acceptLanguage: "ru", want: "ru"},
		{name: "accept language only", acceptLanguage: "ru-RU,en;q=0.5", want: "ru"},
		{name: "neither header", want: DefaultLanguage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RequestLanguage(tt.applicationLanguage, tt.acceptLanguage); got != tt.want {
				t.Fatalf("RequestLanguage(%q, %q) = %q, want %q", tt.applicationLanguage, tt.acceptLanguage, got, tt.want)
			}
		})
	}
}
