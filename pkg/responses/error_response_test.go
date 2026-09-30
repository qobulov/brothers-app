package responses

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func TestErrorShowsTechnicalReasonOnlyInDevelopment(t *testing.T) {
	t.Parallel()

	technicalError := errors.New("ERROR: column email does not exist")

	tests := []struct {
		name        string
		environment string
		language    string
		wantMessage string
		wantReason  string
	}{
		{name: "development Uzbek", environment: "development", language: "uz", wantMessage: "Serverda ichki xatolik yuz berdi", wantReason: technicalError.Error()},
		{
			name:        "production",
			environment: "production",
			wantMessage: "Internal server error",
			wantReason:  "internal_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New()
			Middleware(app, tt.environment)
			app.Get("/", func(c *fiber.Ctx) error {
				return Error(c, technicalError)
			})

			request := httptest.NewRequest("GET", "/", nil)
			request.Header.Set(fiber.HeaderAcceptLanguage, tt.language)
			response, err := app.Test(request)
			require.NoError(t, err)
			defer response.Body.Close()

			var body Envelope[ErrorDetails]
			require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
			require.Equal(t, tt.wantMessage, body.Message)
			require.Equal(t, tt.wantReason, body.Data.Reason)
		})
	}
}

func TestErrorUsesAcceptLanguage(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	Middleware(app, "production")
	app.Get("/", func(c *fiber.Ctx) error {
		return Error(c, apperror.ErrInvalidOTP)
	})

	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set(fiber.HeaderAcceptLanguage, "ru-RU,uz;q=0.8")
	response, err := app.Test(request)
	require.NoError(t, err)
	defer response.Body.Close()

	var body Envelope[any]
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	require.Equal(t, "Неверный или просроченный код", body.Message)
}

func TestErrorWithMessageUsesAcceptLanguage(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	Middleware(app, "production")
	app.Get("/", func(c *fiber.Ctx) error {
		return ErrorWithMessage(c, apperror.ErrInvalidData, MessageInvalidRequest)
	})

	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set(fiber.HeaderAcceptLanguage, "uz")
	response, err := app.Test(request)
	require.NoError(t, err)
	defer response.Body.Close()

	var body Envelope[any]
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	require.Equal(t, "So'rov ma'lumotlari noto'g'ri", body.Message)
}

func TestErrorPreservesCauseAndClassification(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		err     error
		status  int
		code    int
		slug    string
		message string
	}{
		{"missing column", fmt.Errorf("checking registration email: %w", &pgconn.PgError{Severity: "ERROR", Code: "42703", Message: "column email does not exist"}), 500, 1500, "internal_error", "Serverda ichki xatolik yuz berdi"},
		{"duplicate", fmt.Errorf("saving user: %w", &pgconn.PgError{Code: "23505", Message: "duplicate key violates unique constraint users_email_key"}), 409, 1409, "conflict", "Ma'lumotlar ziddiyati"},
		{"db invalid input", &pgconn.PgError{Code: "22P02", Message: "invalid input syntax for type uuid"}, 400, 1400, "invalid_data", "Ma'lumotlar noto'g'ri"},
		{"smtp", fmt.Errorf("%w: %w", apperror.ErrEmailUnavailable, errors.New("authenticating smtp client: 535 authentication failed")), 503, 1503, "email_delivery_unavailable", "Email yuborish vaqtincha ishlamayapti"},
		{"invalid ID", apperror.ErrInvalidID, 400, 1400, "invalid_id", "ID noto'g'ri"},
		{"framework", fiber.NewError(400, "invalid JSON body"), 400, 1400, "invalid_data", "Ma'lumotlar noto'g'ri"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app := fiber.New(fiber.Config{ErrorHandler: Error})
			Middleware(app, "production")
			app.Get("/", func(c *fiber.Ctx) error { return tt.err })
			request := httptest.NewRequest("GET", "/", nil)
			request.Header.Set(fiber.HeaderAcceptLanguage, "uz")
			response, err := app.Test(request)
			require.NoError(t, err)
			defer response.Body.Close()
			var body Envelope[ErrorDetails]
			require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
			require.Equal(t, tt.status, response.StatusCode)
			require.Equal(t, tt.code, body.Code)
			require.Equal(t, tt.slug, body.Slug)
			require.Equal(t, tt.message, body.Message)
			wantReason := tt.err.Error()
			if tt.status >= fiber.StatusInternalServerError {
				// Production hides internal causes; they go to logs and Telegram only.
				wantReason = tt.slug
			}
			require.Equal(t, wantReason, body.Data.Reason)
			require.NotEmpty(t, body.Meta.RequestID)
		})
	}
}

// The mobile app sends Application-Language instead of Accept-Language.
func TestResponsesUseApplicationLanguageHeader(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	Middleware(app, "production")
	app.Get("/error", func(c *fiber.Ctx) error { return Error(c, apperror.ErrInvalidData) })
	app.Get("/message", func(c *fiber.Ctx) error {
		return ErrorWithMessage(c, apperror.ErrInvalidData, MessageInvalidRequest)
	})
	app.Get("/success", func(c *fiber.Ctx) error {
		return Success(c, fiber.StatusOK, "ok", MessageRequestProcessed)
	})

	for path, want := range map[string]string{
		"/error":   "Ma'lumotlar noto'g'ri",
		"/message": "So'rov ma'lumotlari noto'g'ri",
		"/success": "So'rov muvaffaqiyatli bajarildi",
	} {
		request := httptest.NewRequest("GET", path, nil)
		request.Header.Set("Application-Language", "uz")
		request.Header.Set(fiber.HeaderAcceptLanguage, "en")
		response, err := app.Test(request)
		require.NoError(t, err)
		var body struct {
			Message string `json:"message"`
		}
		require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
		response.Body.Close()
		require.Equal(t, want, body.Message, path)
	}
}

func TestErrorDetailsHideInternalReasons(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		environment, wantReason string
	}{
		{environment: "production", wantReason: "internal_error"},
		{environment: "development", wantReason: "loading order: ERROR: syntax error at or near \"FROM\""},
	} {
		var reports []FailureReport
		app := fiber.New()
		Middleware(app, tt.environment, func(report FailureReport) { reports = append(reports, report) })
		app.Get("/internal", func(c *fiber.Ctx) error {
			return Error(c, errors.New(`loading order: ERROR: syntax error at or near "FROM"`))
		})
		app.Get("/field", func(c *fiber.Ctx) error {
			return Error(c, apperror.New(apperror.ErrInvalidData, apperror.Text{UZ: "a", RU: "b", EN: "Bad amount"}))
		})

		response, err := app.Test(httptest.NewRequest("GET", "/internal", nil))
		require.NoError(t, err)
		var internal Envelope[ErrorDetails]
		require.NoError(t, json.NewDecoder(response.Body).Decode(&internal))
		response.Body.Close()
		require.Equal(t, tt.wantReason, internal.Data.Reason, tt.environment)
		require.Len(t, reports, 1)
		require.Equal(t, `loading order: ERROR: syntax error at or near "FROM"`, reports[0].Reason, "Telegram always gets the full reason")

		response, err = app.Test(httptest.NewRequest("GET", "/field", nil))
		require.NoError(t, err)
		var field Envelope[ErrorDetails]
		require.NoError(t, json.NewDecoder(response.Body).Decode(&field))
		response.Body.Close()
		require.Equal(t, "invalid data: Bad amount", field.Data.Reason, "client errors keep their reason in every environment")
	}
}
