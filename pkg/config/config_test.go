package config

import "testing"

func TestLoadConfigPrefersDeploymentEnvironment(t *testing.T) {
	t.Setenv("PORT", "3000")
	t.Setenv("APP_PORT", "8000")
	t.Setenv("DATABASE_URL", "postgresql://user:password@database.example.com:5432/brothers")
	t.Setenv("CORS_ALLOW_ORIGINS", "https://app.example.com")
	t.Setenv("CORS_ALLOW_CREDENTIALS", "true")

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
}
