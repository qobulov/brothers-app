package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/qobulov/brothers-app/pkg/config"
)

func TestFiberMiddlewareCORSPreflight(t *testing.T) {
	app := fiber.New()
	cfg := &config.Config{
		CORSAllowOrigins:     "https://frontend.example.com",
		CORSAllowCredentials: true,
	}
	if err := FiberMiddleware(app, cfg); err != nil {
		t.Fatalf("configure middleware: %v", err)
	}
	app.Post("/api/v1/auth/login", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	request.Header.Set("Origin", "https://frontend.example.com")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "Authorization, Content-Type")
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("send preflight request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want %d", response.StatusCode, http.StatusNoContent)
	}
	if origin := response.Header.Get("Access-Control-Allow-Origin"); origin != "https://frontend.example.com" {
		t.Fatalf("Access-Control-Allow-Origin = %q", origin)
	}
	if credentials := response.Header.Get("Access-Control-Allow-Credentials"); credentials != "true" {
		t.Fatalf("Access-Control-Allow-Credentials = %q", credentials)
	}
}

func TestFiberMiddlewareRejectsWildcardCredentials(t *testing.T) {
	err := FiberMiddleware(fiber.New(), &config.Config{
		CORSAllowOrigins:     "*",
		CORSAllowCredentials: true,
	})
	if err == nil {
		t.Fatal("expected wildcard credentials configuration to fail")
	}
}
