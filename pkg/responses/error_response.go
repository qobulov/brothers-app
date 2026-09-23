package responses

import (
	"github.com/gofiber/fiber/v2"
	appError "github.com/qobulov/brothers-app/pkg/apperror"
)

// ErrorResponse represents the standard error response
type ErrorResponse struct {
	Error string `json:"error" example:"example error"`
}

func Error(c *fiber.Ctx, err error) error {
	return ErrorLocalized(c, err, "")
}

// ErrorLocalized writes an error using the explicit language when provided,
// otherwise it falls back to the request's Accept-Language header.
func ErrorLocalized(c *fiber.Ctx, err error, language string) error {
	if language == "" {
		language = c.Get(fiber.HeaderAcceptLanguage)
	}
	return Failure(c, appError.StatusCode(err), appError.Code(err), appError.Slug(err), appError.MessageForLanguage(err, language), nil)
}

func ErrorWithMessage(c *fiber.Ctx, err error, message string) error {
	return Failure(c, appError.StatusCode(err), appError.Code(err), appError.Slug(err), message, nil)
}
