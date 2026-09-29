package config

import (
	"testing"
	"time"
)

func TestLoadConfigPrefersDeploymentEnvironment(t *testing.T) {
	t.Setenv("PORT", "3000")
	t.Setenv("APP_PORT", "8000")
	t.Setenv("DATABASE_URL", "postgresql://user:password@database.example.com:5432/brothers")
	t.Setenv("CORS_ALLOW_ORIGINS", "https://app.example.com")
	t.Setenv("CORS_ALLOW_CREDENTIALS", "true")
	t.Setenv("TELEGRAM_BACKEND_ERROR_THREAD_ID", "123")

	cfg := LoadConfig("file-that-does-not-exist")

	if cfg.AppPort != "3000" {
		t.Fatalf("AppPort = %q, want Vercel PORT value %q", cfg.AppPort, "3000")
	}
	if cfg.DatabaseDSN != "postgresql://user:password@database.example.com:5432/brothers" {
		t.Fatalf("DatabaseDSN = %q, want DATABASE_URL value", cfg.DatabaseDSN)
	}
	if cfg.CORSAllowOrigins != "https://app.example.com" || !cfg.CORSAllowCredentials {
		t.Fatalf("CORS config = %q, %t", cfg.CORSAllowOrigins, cfg.CORSAllowCredentials)
	}
	if cfg.TelegramBackendErrorThreadID != 123 {
		t.Fatalf("TelegramBackendErrorThreadID = %d, want 123", cfg.TelegramBackendErrorThreadID)
	}
}

func TestLoadConfigUsesFormulaSMTPFromVariable(t *testing.T) {
	t.Setenv("SMTP_FROM", "noreply@example.com")

	cfg := LoadConfig("file-that-does-not-exist")
	if cfg.SMTPFrom != "noreply@example.com" {
		t.Fatalf("SMTPFrom = %q, want Formula SMTP_FROM value", cfg.SMTPFrom)
	}
}

func TestLoadConfigReadsAPIRequestTimeout(t *testing.T) {
	t.Setenv("API_REQUEST_TIMEOUT", "750ms")

	cfg := LoadConfig("file-that-does-not-exist")
	if cfg.APIRequestTimeout != 750*time.Millisecond {
		t.Fatalf("APIRequestTimeout = %s, want 750ms", cfg.APIRequestTimeout)
	}
}
