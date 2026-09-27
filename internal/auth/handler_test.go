package auth

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/qobulov/brothers-app/pkg/responses"
)

func TestMalformedBodyPreservesParseError(t *testing.T) {
	t.Parallel()
	handler := NewHandler(nil)
	app := fiber.New(fiber.Config{ErrorHandler: responses.Error})
	responses.Middleware(app, "production")
	app.Post("/otp", handler.SendOTP)
	request := httptest.NewRequest("POST", "/otp", strings.NewReader(`{"email":`))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	request.Header.Set(fiber.HeaderAcceptLanguage, "uz")
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body responses.Envelope[responses.ErrorDetails]
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 400 || body.Code != 1400 || body.Slug != "invalid_data" {
		t.Fatalf("invalid JSON must return 400/1400/invalid_data: status=%d body=%+v", response.StatusCode, body)
	}
	if body.Message != "So'rov ma'lumotlari noto'g'ri" || !strings.Contains(body.Data.Reason, "unexpected end of JSON input") {
		t.Fatalf("expected localized message and original parse error: %+v", body)
	}
}
