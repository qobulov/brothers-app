package responses

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
)

func TestFailureUsesAcceptLanguage(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	Middleware(app, "production")
	app.Get("/", func(c *fiber.Ctx) error {
		return Failure(c, fiber.StatusNotFound, 1404, "not_found", MessageResourceNotFound, nil)
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

func TestFailureReporterGetsOriginalErrorAndRedactedRequestBody(t *testing.T) {
	var reports []FailureReport
	app := fiber.New(fiber.Config{ErrorHandler: Error})
	Middleware(app, "test", func(report FailureReport) { reports = append(reports, report) })
	app.Post("/items/:id", func(c *fiber.Ctx) error { return errors.New("database connection refused") })
	request := httptest.NewRequest("POST", "/items/private-id?token=secret", strings.NewReader(`{"email":"ali@example.com","username":"qobulov","purpose":"registration","password":"secret","nested":{"refresh_token":"token-value"}}`))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	request.Header.Set("Authorization", "Bearer secret")
	request.Header.Set("X-Request-ID", "test-request-123")
	response, err := app.Test(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Len(t, reports, 1)
	require.Equal(t, "/items/:id", reports[0].Path)
	require.Equal(t, "database connection refused", reports[0].Reason)
	require.JSONEq(t, `{"email":"[REDACTED]","username":"[REDACTED]","purpose":"registration","password":"[REDACTED]","nested":{"refresh_token":"[REDACTED]"}}`, reports[0].RequestBody)
	require.Equal(t, "test-request-123", reports[0].Meta.RequestID)
	require.Equal(t, 500, reports[0].Status)
	var body Envelope[ErrorDetails]
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	require.Equal(t, reports[0].Meta, body.Meta)
	encodedBody, err := json.Marshal(body)
	require.NoError(t, err)
	require.JSONEq(t, string(encodedBody), reports[0].ResponseBody)
}

func TestFailureReporterOmitsInvalidJSONBody(t *testing.T) {
	var report FailureReport
	app := fiber.New()
	Middleware(app, "test", func(value FailureReport) { report = value })
	app.Post("/items", func(c *fiber.Ctx) error {
		return Failure(c, fiber.StatusInternalServerError, 1500, "internal_error", "internal error", nil)
	})
	request := httptest.NewRequest("POST", "/items", strings.NewReader(`{"password":"secret"`))
	response, err := app.Test(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, "<request body omitted: invalid JSON>", report.RequestBody)
}

func TestFailureReporterKeepsFullRequestBodyInDevelopment(t *testing.T) {
	var report FailureReport
	app := fiber.New()
	Middleware(app, "development", func(value FailureReport) { report = value })
	app.Post("/items", func(c *fiber.Ctx) error {
		return Failure(c, fiber.StatusInternalServerError, 1500, "internal_error", "internal error", nil)
	})
	const requestBody = `{"email":"ali@example.com","username":"qobulov","purpose":"registration","password":"secret"}`
	request := httptest.NewRequest("POST", "/items", strings.NewReader(requestBody))
	response, err := app.Test(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.JSONEq(t, requestBody, report.RequestBody)
}

func TestFailureReporterIgnoresClientErrors(t *testing.T) {
	var reports []FailureReport
	app := fiber.New()
	Middleware(app, "development", func(report FailureReport) { reports = append(reports, report) })
	for _, status := range []int{
		fiber.StatusBadRequest, fiber.StatusUnauthorized, fiber.StatusForbidden,
		fiber.StatusNotFound, fiber.StatusConflict, fiber.StatusTooManyRequests,
	} {
		app.Get("/status/"+strconv.Itoa(status), func(c *fiber.Ctx) error {
			return Failure(c, status, 1000+status, "client_error", "client error", nil)
		})
		response, err := app.Test(httptest.NewRequest("GET", "/status/"+strconv.Itoa(status), nil))
		require.NoError(t, err)
		response.Body.Close()
		require.Equal(t, status, response.StatusCode)
	}
	require.Empty(t, reports, "client errors must not be reported")
}
