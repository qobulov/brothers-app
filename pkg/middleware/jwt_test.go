package middleware

import "testing"

func TestAuthorizationToken(t *testing.T) {
	const token = "header.payload.signature"
	tests := []struct {
		name   string
		header string
		want   string
		ok     bool
	}{
		{name: "bearer", header: "Bearer " + token, want: token, ok: true},
		{name: "case insensitive bearer", header: "bearer " + token, want: token, ok: true},
		{name: "swagger raw token", header: token, want: token, ok: true},
		{name: "missing", header: ""},
		{name: "wrong scheme", header: "Basic " + token},
		{name: "arbitrary raw value", header: "not-a-jwt"},
		{name: "extra fields", header: "Bearer " + token + " extra"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := authorizationToken(test.header)
			if got != test.want || ok != test.ok {
				t.Fatalf("authorizationToken(%q) = (%q, %v), want (%q, %v)", test.header, got, ok, test.want, test.ok)
			}
		})
	}
}
