package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/internal/auth/session"
	"github.com/qobulov/brothers-app/pkg/config"
	"github.com/qobulov/brothers-app/pkg/responses"
)

type sessionStoreFailure struct{ err error }

func (s sessionStoreFailure) Create(context.Context, session.Data, time.Duration) error { return s.err }
func (s sessionStoreFailure) Validate(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return false, s.err
}
func (s sessionStoreFailure) Rotate(context.Context, string, string, time.Duration) (session.Data, error) {
	return session.Data{}, s.err
}
func (s sessionStoreFailure) Revoke(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return false, s.err
}
func (s sessionStoreFailure) RevokeUser(context.Context, uuid.UUID) error { return s.err }

func TestSessionJWTReportsRedisFailure(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{JWTSecret: "test-secret", JWTIssuer: "test", JWTAudience: "test"}
	claims := jwt.MapClaims{
		"sub": uuid.NewString(), "sid": uuid.NewString(), "type": "access",
		"iss": cfg.JWTIssuer, "aud": cfg.JWTAudience, "exp": time.Now().Add(time.Hour).Unix(),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		cause  error
		status int
	}{
		{"missing session", session.ErrNotFound, 401},
		{"redis failure", errors.New("redis connection refused"), 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			responses.Middleware(app, "production")
			app.Get("/me", SessionJWTMiddleware(sessionStoreFailure{err: tt.cause}, cfg), func(c *fiber.Ctx) error { return c.SendStatus(200) })
			request := httptest.NewRequest("GET", "/me", nil)
			request.Header.Set("Authorization", "Bearer "+token)
			response, err := app.Test(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != tt.status {
				t.Fatalf("status = %d, want %d", response.StatusCode, tt.status)
			}
			var body responses.Envelope[responses.ErrorDetails]
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if tt.status == 500 && !strings.Contains(body.Data.Reason, tt.cause.Error()) {
				t.Fatalf("DB cause lost: %+v", body)
			}
		})
	}
}

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
