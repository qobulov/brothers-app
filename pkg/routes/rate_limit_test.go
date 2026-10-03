package routes_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qobulov/brothers-app/internal/app"
	"github.com/qobulov/brothers-app/internal/auth/otp"
	"github.com/qobulov/brothers-app/internal/auth/session"
	"github.com/qobulov/brothers-app/pkg/cache"
	"github.com/qobulov/brothers-app/pkg/config"
	"github.com/qobulov/brothers-app/pkg/database"
)

func TestLogin_IsLimitedPerAccount(t *testing.T) {
	client := cache.TestClient(t, 13)
	pool, cleanup := database.SetupTestDB(t)
	t.Cleanup(cleanup)
	cfg := config.LoadConfig("dev")
	cfg.TelegramBotToken = ""
	server, err := app.SetupRestServer(pool, otp.NewCache(client), session.NewMemoryStore(), nil, cfg)
	if err != nil {
		t.Fatalf("setup server: %v", err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })

	login := func(name string) (int, string) {
		t.Helper()
		body := `{"login":"` + name + `","password":"WrongPass123"}`
		request := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Forwarded-For", "203.0.113.7")
		request.Header.Set("Application-Language", "en")
		response, err := server.Test(request, -1)
		if err != nil {
			t.Fatalf("login request: %v", err)
		}
		defer response.Body.Close()
		var envelope struct {
			Slug    string `json:"slug"`
			Message string `json:"message"`
		}
		if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return response.StatusCode, envelope.Slug + ": " + envelope.Message
	}

	for attempt := 1; attempt <= 10; attempt++ {
		if status, _ := login("victim"); status != 401 {
			t.Fatalf("attempt %d: status %d, want 401", attempt, status)
		}
	}
	status, message := login("VICTIM")
	if status != 429 || message != "rate_limit_exceeded: Too many requests. Please try again later" {
		t.Fatalf("attempt 11: got %d %q, want 429 with the rate limit message", status, message)
	}
	if status, _ := login("someone_else"); status != 401 {
		t.Fatalf("another account from the same address: status %d, want 401", status)
	}
}
