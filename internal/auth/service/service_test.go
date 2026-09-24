package service

import (
	"context"
	"errors"
	"testing"

	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/config"
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

func TestConfiguredOTP(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		code     string
		expected string
	}{
		{name: "development fixed code", env: "development", code: "111111", expected: "111111"},
		{name: "trim fixed code", env: "test", code: " 111111 ", expected: "111111"},
		{name: "production fixed code", env: "production", code: "111111", expected: "111111"},
		{name: "empty production code ignored", env: "production"},
		{name: "invalid code ignored", env: "development", code: "12345"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &Service{cfg: &config.Config{AppEnv: test.env, OTPDefaultCode: test.code}}
			if got := service.configuredOTP(); got != test.expected {
				t.Fatalf("configuredOTP() = %q, want %q", got, test.expected)
			}
		})
	}
}

func TestConsumeOTPAcceptsConfiguredCodeWithoutCachedFlow(t *testing.T) {
	service := &Service{cfg: &config.Config{OTPDefaultCode: "111111"}}

	if err := service.consumeOTP(context.Background(), registrationPurpose, "+998930693005", "111111"); err != nil {
		t.Fatalf("consumeOTP() error = %v, want nil", err)
	}
}

func TestNormalizeOTPPurpose(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "defaults to registration", want: registrationPurpose},
		{name: "registration", input: " registration ", want: registrationPurpose},
		{name: "password reset", input: " PASSWORD_RESET ", want: passwordResetPurpose},
		{name: "unsupported purpose", input: "login", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeOTPPurpose(test.input)
			if test.wantErr {
				if !errors.Is(err, apperror.ErrInvalidData) {
					t.Fatalf("normalizeOTPPurpose() error = %v, want ErrInvalidData", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeOTPPurpose() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("normalizeOTPPurpose() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestLoginIdentifiers(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantUsername string
		wantPhone    string
	}{
		{name: "username", input: "  qobulov  ", wantUsername: "qobulov"},
		{name: "phone", input: "+998 90 123 45 67", wantUsername: "+998 90 123 45 67", wantPhone: "+998901234567"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			username, phone := loginIdentifiers(test.input)
			if username != test.wantUsername {
				t.Errorf("username = %q, want %q", username, test.wantUsername)
			}
			if phone != test.wantPhone {
				t.Errorf("phone = %q, want %q", phone, test.wantPhone)
			}
		})
	}
}

func TestOptionalProfileFields(t *testing.T) {
	validName := "  Qobul  "
	validLanguage := " RU "
	validAvatar := "https://example.com/avatar.jpg"
	emptyAvatar := ""
	invalidName := "   "
	invalidLanguage := "de"
	invalidAvatar := "javascript:alert(1)"

	tests := []struct {
		name    string
		run     func() error
		wantErr bool
	}{
		{name: "trim name", run: func() error {
			value, err := optionalName(&validName)
			if value.String != "Qobul" {
				t.Errorf("name = %q", value.String)
			}
			return err
		}},
		{name: "accept language", run: func() error {
			value, err := optionalLanguage(&validLanguage)
			if value.String != "ru" {
				t.Errorf("language = %q", value.String)
			}
			return err
		}},
		{name: "accept https avatar", run: func() error { _, err := optionalAvatarURL(&validAvatar); return err }},
		{name: "allow clearing avatar", run: func() error {
			value, err := optionalAvatarURL(&emptyAvatar)
			if !value.Valid || value.String != "" {
				t.Errorf("avatar = %#v", value)
			}
			return err
		}},
		{name: "reject empty name", run: func() error { _, err := optionalName(&invalidName); return err }, wantErr: true},
		{name: "reject unsupported language", run: func() error { _, err := optionalLanguage(&invalidLanguage); return err }, wantErr: true},
		{name: "reject unsafe avatar scheme", run: func() error { _, err := optionalAvatarURL(&invalidAvatar); return err }, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.run()
			if test.wantErr && !errors.Is(err, apperror.ErrInvalidData) {
				t.Fatalf("error = %v, want ErrInvalidData", err)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
