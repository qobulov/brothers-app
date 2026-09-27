package responses

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
)

func TestFailureUsesAcceptLanguage(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	Middleware(app, "production")
	app.Get("/", func(c *fiber.Ctx) error {
		return Failure(c, fiber.StatusNotFound, 1404, "not_found", "Ресурс не найден", nil)
	})

	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set(fiber.HeaderAcceptLanguage, "en-US")
	response, err := app.Test(request)
	require.NoError(t, err)
	defer response.Body.Close()

	var body Envelope[any]
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	require.Equal(t, "Resource not found", body.Message)
}

func TestFailureReporterGetsOriginalErrorWithoutRequestSecrets(t *testing.T) {
	var reports []FailureReport
	app := fiber.New(fiber.Config{ErrorHandler: Error})
	Middleware(app, "test", func(report FailureReport) { reports = append(reports, report) })
	app.Post("/items/:id", func(c *fiber.Ctx) error { return errors.New("database connection refused") })
	request := httptest.NewRequest("POST", "/items/private-id?token=secret", nil)
	request.Header.Set("Authorization", "Bearer secret")
	request.Header.Set("X-Request-ID", "test-request-123")
	response, err := app.Test(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Len(t, reports, 1)
	require.Equal(t, "/items/:id", reports[0].Path)
	require.Equal(t, "database connection refused", reports[0].Reason)
	require.Equal(t, "test-request-123", reports[0].Meta.RequestID)
	require.Equal(t, 500, reports[0].Status)
	var body Envelope[ErrorDetails]
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	require.Equal(t, reports[0].Meta, body.Meta)
}
