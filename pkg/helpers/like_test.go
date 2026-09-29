package helpers

import "testing"

func TestEscapeLike(t *testing.T) {
	tests := map[string]string{
		"":           "",
		"john":       "john",
		"100%":       `100\%`,
		"first_name": `first\_name`,
		`a\b`:        `a\\b`,
	}
	for input, want := range tests {
		if got := EscapeLike(input); got != want {
			t.Errorf("EscapeLike(%q) = %q, want %q", input, got, want)
		}
	}
}
