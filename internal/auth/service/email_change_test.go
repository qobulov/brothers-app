package service

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	dto "github.com/qobulov/brothers-app/internal/auth/dto"
	"github.com/qobulov/brothers-app/internal/auth/otp"
	"github.com/qobulov/brothers-app/internal/auth/session"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/config"
	"github.com/qobulov/brothers-app/pkg/database"
	"github.com/redis/go-redis/v9"
)

// capturingSender records the last code sent to each address instead of mailing it.
type capturingSender struct {
	mu    sync.Mutex
	codes map[string]string
}

func (s *capturingSender) SendOTP(_ context.Context, recipient, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.codes[recipient] = code
	return nil
}

func (s *capturingSender) code(recipient string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.codes[recipient]
}

type emailFixture struct {
	t       *testing.T
	pool    *pgxpool.Pool
	service *Service
	sender  *capturingSender
}

// newEmailFixture needs PostgreSQL and Redis. Without Redis the tests are skipped.
func newEmailFixture(t *testing.T) *emailFixture {
	t.Helper()
	redisURL := os.Getenv("REDIS_TEST_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/15"
	}
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatalf("parse REDIS_TEST_URL: %v", err)
	}
	client := redis.NewClient(options)
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Skipf("redis is not available at %s: %v", redisURL, err)
	}
	if err := client.FlushDB(context.Background()).Err(); err != nil {
		t.Fatalf("flush test redis: %v", err)
	}

	pool, cleanup := database.SetupTestDB(t)
	t.Cleanup(cleanup)
	sender := &capturingSender{codes: map[string]string{}}
	cfg := &config.Config{OTPExpiration: 120, OTPResendCooldown: 60, OTPMaxAttempts: 5, OTPPepper: "test-pepper", AppEnv: "test"}
	return &emailFixture{
		t: t, pool: pool, sender: sender,
		service: New(pool, otp.NewCache(client), session.NewMemoryStore(), cfg, sender),
	}
}

func (f *emailFixture) user(email string) uuid.UUID {
	f.t.Helper()
	var id uuid.UUID
	err := f.pool.QueryRow(context.Background(), `
		INSERT INTO users (email, username, language, is_active) VALUES ($1, $2, 'uz', true) RETURNING id
	`, email, "user_"+uuid.NewString()[:8]).Scan(&id)
	if err != nil {
		f.t.Fatalf("create user: %v", err)
	}
	return id
}

func (f *emailFixture) email(userID uuid.UUID) string {
	f.t.Helper()
	var email string
	if err := f.pool.QueryRow(context.Background(), `SELECT email FROM users WHERE id = $1`, userID).Scan(&email); err != nil {
		f.t.Fatalf("read email: %v", err)
	}
	return email
}

func TestChangeEmail_WithCodeSentToNewAddress(t *testing.T) {
	f := newEmailFixture(t)
	ctx := context.Background()
	userID := f.user("old@example.com")

	started, err := f.service.SendEmailChangeOTP(ctx, userID, " New@Example.com ")
	if err != nil {
		t.Fatalf("send email change otp: %v", err)
	}
	if started.Email != "new@example.com" {
		t.Fatalf("otp sent to %q, want the new address", started.Email)
	}
	code := f.sender.code("new@example.com")
	if code == "" || f.sender.code("old@example.com") != "" {
		t.Fatal("the code must go to the new address only")
	}

	if _, err := f.service.ChangeEmail(ctx, userID, dto.ChangeEmailRequest{NewEmail: "new@example.com", OTPCode: wrongCode(code)}); !errors.Is(err, apperror.ErrInvalidOTP) {
		t.Fatalf("wrong code error = %v, want invalid otp", err)
	}
	if f.email(userID) != "old@example.com" {
		t.Fatal("a wrong code must not change the email")
	}

	user, err := f.service.ChangeEmail(ctx, userID, dto.ChangeEmailRequest{NewEmail: "NEW@example.com", OTPCode: code})
	if err != nil {
		t.Fatalf("change email: %v", err)
	}
	if user.Email != "new@example.com" || f.email(userID) != "new@example.com" {
		t.Fatalf("email after change = %q / %q", user.Email, f.email(userID))
	}
	if _, err := f.service.ChangeEmail(ctx, userID, dto.ChangeEmailRequest{NewEmail: "new@example.com", OTPCode: code}); err == nil {
		t.Fatal("a used code must not work twice")
	}
}

func TestChangeEmail_CodeIsBoundToRequester(t *testing.T) {
	f := newEmailFixture(t)
	ctx := context.Background()
	owner := f.user("owner@example.com")
	attacker := f.user("attacker@example.com")

	if _, err := f.service.SendEmailChangeOTP(ctx, owner, "target@example.com"); err != nil {
		t.Fatalf("send: %v", err)
	}
	code := f.sender.code("target@example.com")
	if _, err := f.service.ChangeEmail(ctx, attacker, dto.ChangeEmailRequest{NewEmail: "target@example.com", OTPCode: code}); !errors.Is(err, apperror.ErrInvalidOTP) {
		t.Fatalf("another user using the code error = %v, want invalid otp", err)
	}
	if _, err := f.service.ChangeEmail(ctx, owner, dto.ChangeEmailRequest{NewEmail: "never-requested@example.com", OTPCode: code}); !errors.Is(err, apperror.ErrInvalidOTP) {
		t.Fatalf("code for an address never requested error = %v, want invalid otp", err)
	}
}

func TestSendEmailChangeOTP_Rules(t *testing.T) {
	f := newEmailFixture(t)
	ctx := context.Background()
	userID := f.user("me@example.com")
	f.user("taken@example.com")

	tests := []struct {
		name     string
		newEmail string
		wantErr  error
	}{
		{name: "invalid address", newEmail: "not-an-email", wantErr: apperror.ErrInvalidData},
		{name: "same as current", newEmail: "ME@example.com", wantErr: apperror.ErrInvalidData},
		{name: "used by another account", newEmail: "taken@example.com", wantErr: apperror.ErrAlreadyExists},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := f.service.SendEmailChangeOTP(ctx, userID, tt.newEmail); !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
	if _, err := f.service.SendEmailChangeOTP(ctx, userID, "fresh@example.com"); err != nil {
		t.Fatalf("first send: %v", err)
	}
	if _, err := f.service.SendEmailChangeOTP(ctx, userID, "fresh@example.com"); !errors.Is(err, apperror.ErrLimitExceeded) {
		t.Fatalf("resend within cooldown error = %v, want limit exceeded", err)
	}
	if _, err := f.service.SendEmailChangeOTP(ctx, uuid.New(), "other@example.com"); !errors.Is(err, apperror.ErrUnauthorized) {
		t.Fatalf("unknown user error = %v, want unauthorized", err)
	}
}

// wrongCode returns a valid-looking code that differs from code and from the
// default test code.
func wrongCode(code string) string {
	for _, candidate := range []string{"000000", "000001", "000002"} {
		if candidate != code && candidate != defaultOTP {
			return candidate
		}
	}
	return "999999"
}
