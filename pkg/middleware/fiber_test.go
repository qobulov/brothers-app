package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/qobulov/brothers-app/pkg/config"
	"github.com/qobulov/brothers-app/pkg/responses"
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

func TestRequestTimeoutReturnsStandardGatewayTimeout(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: responses.Error})
	responses.Middleware(app, "test")
	app.Use(RequestTimeout(10 * time.Millisecond))
	app.Get("/slow", func(c *fiber.Ctx) error {
		<-c.UserContext().Done()
		return c.UserContext().Err()
	})

	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/slow", nil))
	if err != nil {
		t.Fatalf("send slow request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusGatewayTimeout)
	}
	var body responses.Envelope[responses.ErrorDetails]
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode timeout response: %v", err)
	}
	if body.Slug != "timeout" || body.Code != 1504 {
		t.Fatalf("timeout response = code %d slug %q, want code 1504 slug timeout", body.Code, body.Slug)
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
