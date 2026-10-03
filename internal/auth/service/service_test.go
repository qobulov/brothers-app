package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	authdto "github.com/qobulov/brothers-app/internal/auth/dto"
	"github.com/qobulov/brothers-app/internal/auth/otp"
	db "github.com/qobulov/brothers-app/internal/db"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/config"
	"github.com/qobulov/brothers-app/pkg/helpers"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

type failedUserLookup struct {
	db.DBTX
	err error
}

func (q failedUserLookup) QueryRow(context.Context, string, ...interface{}) pgx.Row {
	return failedUserRow{err: q.err}
}

type failedUserRow struct{ err error }

func (r failedUserRow) Scan(...interface{}) error { return r.err }

func TestSendOTPRejectsUnknownPasswordResetAccount(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		request authdto.SendOTPRequest
	}{
		{name: "email", request: authdto.SendOTPRequest{Email: "random@example.com", Purpose: passwordResetPurpose}},
		{name: "username", request: authdto.SendOTPRequest{Username: "random_user", Purpose: passwordResetPurpose}},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := &Service{
				queries: db.New(failedUserLookup{err: pgx.ErrNoRows}),
				cfg:     &config.Config{OTPExpiration: 300},
				now:     time.Now,
			}
			// No cache or sender: missing accounts must stop before starting an OTP flow.
			data, err := s.SendOTP(t.Context(), test.request)
			if !errors.Is(err, apperror.ErrRecordNotFound) {
				t.Fatalf("SendOTP() = (%+v, %v), want account not found", data, err)
			}
			if data != (authdto.StartData{}) {
				t.Fatalf("SendOTP() returned OTP metadata for a missing account: %+v", data)
			}
			if apperror.StatusCode(err) != 404 || apperror.MessageForLanguage(err, "uz") != "Bu email yoki username bilan akkaunt topilmadi" {
				t.Fatalf("unexpected account not found response: %v", err)
			}
		})
	}
}

func TestSendOTPPreservesPasswordResetLookupError(t *testing.T) {
	t.Parallel()
	cause := errors.New("database unavailable")
	s := &Service{queries: db.New(failedUserLookup{err: cause})}
	_, err := s.SendOTP(t.Context(), authdto.SendOTPRequest{Email: "ali@example.com", Purpose: passwordResetPurpose})
	if !errors.Is(err, cause) || errors.Is(err, apperror.ErrRecordNotFound) {
		t.Fatalf("SendOTP() = %v, want original database error", err)
	}
}

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
	err = s.ResetPassword(context.Background(), authdto.ResetPasswordRequest{Password: "TestPass123", ResetToken: "test-token"})
	if !errors.Is(err, cause) || errors.Is(err, apperror.ErrInvalidResetToken) {
		t.Fatalf("ResetPassword() = %v, want original Redis error", err)
	}
}

func TestDefaultOTPSkipsCacheVerification(t *testing.T) {
	s := &Service{otp: otp.NewCache(failingOTPCache{err: errors.New("redis unavailable")}), cfg: &config.Config{}}
	if err := s.consumeOTP(context.Background(), registrationPurpose, "ali@example.com", defaultOTP); err != nil {
		t.Fatalf("consumeOTP() = %v, want default OTP accepted without Redis", err)
	}
}

func TestPasswordResetUserNotFoundReasonDependsOnEnvironment(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		wantDetail  bool
	}{
		{name: "development includes exact reason", environment: "development", wantDetail: true},
		{name: "production keeps generic reason", environment: "production"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := &Service{cfg: &config.Config{AppEnv: test.environment}}
			err := s.passwordResetUserNotFound("ali@example.com")
			if !errors.Is(err, apperror.ErrInvalidOTP) {
				t.Fatalf("passwordResetUserNotFound() = %v, want ErrInvalidOTP", err)
			}
			hasDetail := strings.Contains(err.Error(), `password reset user not found for email "ali@example.com"`)
			if hasDetail != test.wantDetail {
				t.Fatalf("passwordResetUserNotFound() detail = %t, want %t; error = %q", hasDetail, test.wantDetail, err)
			}
		})
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

func TestSendOTPRejectsUsernameForRegistrationWithReason(t *testing.T) {
	s := &Service{}
	_, err := s.SendOTP(t.Context(), authdto.SendOTPRequest{
		Email: "ali@example.com", Username: "qobulov", Purpose: registrationPurpose,
	})
	if !errors.Is(err, apperror.ErrInvalidData) {
		t.Fatalf("SendOTP() error = %v, want ErrInvalidData", err)
	}
	if want := "Registration accepts email only; omit the username"; !strings.Contains(err.Error(), want) {
		t.Fatalf("SendOTP() error = %q, want reason %q", err, want)
	}
}

func TestPasswordResetIdentifier(t *testing.T) {
	tests := []struct {
		name         string
		request      authdto.SendOTPRequest
		wantEmail    string
		wantUsername string
		wantReason   string
	}{
		{name: "email", request: authdto.SendOTPRequest{Email: " Ali@Example.COM "}, wantEmail: "ali@example.com"},
		{name: "username", request: authdto.SendOTPRequest{Username: " qobulov "}, wantUsername: "qobulov"},
		{name: "both identifiers", request: authdto.SendOTPRequest{Email: "ali@example.com", Username: "qobulov"}, wantReason: "Provide either an email or a username, not both"},
		{name: "missing identifier", request: authdto.SendOTPRequest{}, wantReason: "Enter an email or a username to reset the password"},
		{name: "invalid email", request: authdto.SendOTPRequest{Email: "invalid"}, wantReason: "Enter a valid email address"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			email, username, err := passwordResetIdentifier(test.request)
			if test.wantReason != "" {
				if !errors.Is(err, apperror.ErrInvalidData) {
					t.Fatalf("passwordResetIdentifier() error = %v, want ErrInvalidData", err)
				}
				if !strings.Contains(err.Error(), test.wantReason) {
					t.Fatalf("passwordResetIdentifier() error = %q, want reason %q", err, test.wantReason)
				}
				return
			}
			if err != nil {
				t.Fatalf("passwordResetIdentifier() error = %v", err)
			}
			if email != test.wantEmail || username != test.wantUsername {
				t.Fatalf("passwordResetIdentifier() = (%q, %q), want (%q, %q)", email, username, test.wantEmail, test.wantUsername)
			}
		})
	}
}

func TestStartDataIncludesRecipientEmail(t *testing.T) {
	s := &Service{cfg: &config.Config{OTPExpiration: 300, OTPResendCooldown: 60}}
	data := s.startData(time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC), "ali@example.com")
	if data.Email != "ali@example.com" {
		t.Fatalf("StartData.Email = %q, want recipient email", data.Email)
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
		{name: "username in mixed case", input: " Qobulov ", wantUsername: "Qobulov"},
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
			value, err := optionalName(&validName, "first_name")
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
		{name: "reject empty name", run: func() error { _, err := optionalName(&invalidName, "first_name"); return err }, wantErr: true},
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

func TestDummyPasswordHashCostsAsMuchAsRealHashes(t *testing.T) {
	cost, err := bcrypt.Cost(dummyPasswordHash)
	if err != nil {
		t.Fatalf("dummy hash is not a bcrypt hash: %v", err)
	}
	if cost != bcrypt.DefaultCost {
		t.Fatalf("dummy hash cost = %d, want %d", cost, bcrypt.DefaultCost)
	}
	if err := bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte("password")); !errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		t.Fatalf("compare error = %v, want mismatch", err)
	}
}
