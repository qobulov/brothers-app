package responses

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/qobulov/brothers-app/pkg/apperror"
	appError "github.com/qobulov/brothers-app/pkg/apperror"
)

// ErrorResponse represents the standard error response
type ErrorResponse struct {
	Error string `json:"error" example:"example error"`
}

// ErrorDetails carries the original cause, independently of the translated message.
type ErrorDetails struct {
	// Reason is a technical explanation for developers, always in English.
	// Outside development, server errors report only their slug here.
	Reason string `json:"reason" example:"invalid data: The amount must be between $1 and $1000000000"`
}

func Error(c *fiber.Ctx, err error) error {
	return ErrorLocalized(c, err, "")
}

// ErrorLocalized writes an error using the explicit language when provided,
// otherwise it uses the request's language headers.
func ErrorLocalized(c *fiber.Ctx, err error, language string) error {
	if err == nil {
		err = appError.ErrInternalServer
	}
	if language == "" {
		language = Language(c)
	}

	status := appError.StatusCode(err)
	return writeFailure(c, failure{
		status: status, code: appError.Code(err), slug: appError.Slug(err),
		message: appError.MessageForLanguage(err, language), data: errorDetails(c, status, err), reason: err.Error(),
	})
}

func ErrorWithMessage(c *fiber.Ctx, err error, message string) error {
	if err == nil {
		err = appError.ErrInternalServer
	}
	status := appError.StatusCode(err)
	return writeFailure(c, failure{
		status: status, code: appError.Code(err), slug: appError.Slug(err),
		message: message, data: errorDetails(c, status, err), reason: err.Error(),
	})
}

// errorDetails keeps internal causes (SQL, network) out of the response body
// outside development; the full reason still reaches logs and Telegram.
func errorDetails(c *fiber.Ctx, status int, err error) ErrorDetails {
	details := ErrorDetails{Reason: err.Error()}
	environment, _ := c.Locals(appEnvironmentLocal).(string)
	if status >= fiber.StatusInternalServerError && environment != "development" {
		details.Reason = appError.Slug(err)
	}
	return details
}

// InvalidBody reports a request body that could not be parsed.
func InvalidBody(c *fiber.Ctx, err error) error {
	return ErrorWithMessage(c, fmt.Errorf("%w: %w", apperror.ErrInvalidData, err), MessageInvalidRequest)
}
