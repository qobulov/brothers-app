package responses

import (
	"github.com/gofiber/fiber/v2"
	appError "github.com/qobulov/brothers-app/pkg/apperror"
)

// ErrorResponse represents the standard error response
type ErrorResponse struct {
	Error string `json:"error" example:"example error"`
}

// ErrorDetails carries the original cause, independently of the translated message.
type ErrorDetails struct {
	Reason string `json:"reason" example:"checking registration email: ERROR: column email does not exist (SQLSTATE 42703)"`
}

func Error(c *fiber.Ctx, err error) error {
	return ErrorLocalized(c, err, "")
}

// ErrorLocalized writes an error using the explicit language when provided,
// otherwise it falls back to the request's Accept-Language header.
func ErrorLocalized(c *fiber.Ctx, err error, language string) error {
	if err == nil {
		err = appError.ErrInternalServer
	}
	if language == "" {
		language = c.Get(fiber.HeaderAcceptLanguage)
	}

	message := appError.MessageForLanguage(err, language)
	if c.Locals(appEnvironmentLocal) == "development" && appError.Code(err) == 1500 {
		message = err.Error()
	}

	return Failure(c, appError.StatusCode(err), appError.Code(err), appError.Slug(err), message, ErrorDetails{Reason: err.Error()})
}

func ErrorWithMessage(c *fiber.Ctx, err error, message string) error {
	if err == nil {
		err = appError.ErrInternalServer
	}
	return Failure(c, appError.StatusCode(err), appError.Code(err), appError.Slug(err), message, ErrorDetails{Reason: err.Error()})
}
