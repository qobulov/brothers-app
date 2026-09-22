package service

import (
	"testing"

	"github.com/qobulov/brothers-app/pkg/helpers"
)

func TestNormalizePhone(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{name: "uzbek phone", input: "+998 90 123 45 67", expected: "+998901234567"},
		{name: "local digits", input: "998901234567", expected: "+998901234567"},
		{name: "invalid characters", input: "+99890abc", wantErr: true},
		{name: "too short", input: "123", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := helpers.NormalizePhone(test.input)
			if test.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != test.expected {
				t.Fatalf("expected %q, got %q", test.expected, got)
			}
		})
	}
}

func TestGenerateOTP(t *testing.T) {
	for i := 0; i < 100; i++ {
		value, err := helpers.GenerateOTP()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !helpers.ValidOTP(value) {
			t.Fatalf("generated invalid otp %q", value)
		}
	}
}
