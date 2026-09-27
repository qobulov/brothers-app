package service

import (
	"context"
	"errors"
	"testing"

	"github.com/qobulov/brothers-app/internal/auth/dto"
	"github.com/qobulov/brothers-app/internal/auth/otp"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/config"
	"github.com/qobulov/brothers-app/pkg/helpers"
	"github.com/redis/go-redis/v9"
)

// Only the commands used by these error paths are implemented; no Redis server is needed.
type failingOTPCache struct {
	redis.Cmdable
	err error
}

func (c failingOTPCache) Eval(context.Context, string, []string, ...interface{}) *redis.Cmd {
	return redis.NewCmdResult(nil, c.err)
}

func (c failingOTPCache) GetDel(context.Context, string) *redis.StringCmd {
	return redis.NewStringResult("", c.err)
}

func TestPasswordFlowsPreserveCacheErrors(t *testing.T) {
	t.Parallel()
	cause := errors.New("redis connection refused")
	s := &Service{otp: otp.NewCache(failingOTPCache{err: cause}), cfg: &config.Config{OTPMaxAttempts: 5}}
	_, err := s.VerifyPasswordOTP(context.Background(), "ali@example.com", "123456")
	if !errors.Is(err, cause) || errors.Is(err, apperror.ErrInvalidOTP) {
		t.Fatalf("VerifyPasswordOTP() = %v, want original Redis error", err)
	}
	err = s.ResetPassword(context.Background(), authdto.ResetPasswordRequest{Password: "test-password", ResetToken: "test-token"})
	if !errors.Is(err, cause) || errors.Is(err, apperror.ErrInvalidResetToken) {
		t.Fatalf("ResetPassword() = %v, want original Redis error", err)
	}
}

func TestNormalizeEmail(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{name: "lowercases and trims", input: " Ali@Example.COM ", expected: "ali@example.com"},
		{name: "missing domain", input: "ali@localhost", wantErr: true},
		{name: "display name", input: "Ali <ali@example.com>", wantErr: true},
		{name: "invalid characters", input: "ali@@example.com", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := helpers.NormalizeEmail(test.input)
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
	service := &Service{}
	generated := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		value, err := service.generateOTP()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !helpers.ValidOTP(value) {
			t.Fatalf("generated invalid otp %q", value)
		}
		generated[value] = struct{}{}
	}
	if len(generated) == 1 {
		t.Fatal("generateOTP() returned the same fixed code 100 times")
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
		wantEmail    string
	}{
		{name: "username", input: "  qobulov  ", wantUsername: "qobulov"},
		{name: "email", input: " Ali@Example.COM ", wantUsername: "Ali@Example.COM", wantEmail: "ali@example.com"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			username, email := loginIdentifiers(test.input)
			if username != test.wantUsername {
				t.Errorf("username = %q, want %q", username, test.wantUsername)
			}
			if email != test.wantEmail {
				t.Errorf("email = %q, want %q", email, test.wantEmail)
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
