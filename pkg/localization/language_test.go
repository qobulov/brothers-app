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
