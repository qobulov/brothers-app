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
	return Failure(c, appError.StatusCode(err), appError.Code(err), appError.Slug(err), appError.Message(err), nil)
}

func ErrorWithMessage(c *fiber.Ctx, err error, message string) error {
	return Failure(c, appError.StatusCode(err), appError.Code(err), appError.Slug(err), message, nil)
}
