package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/responses"
)

// RegisterNotFoundRoute sets a custom handler for 404 Not Found
func RegisterNotFoundRoute(app *fiber.App) {
	app.Use(func(c *fiber.Ctx) error {
		return responses.ErrorWithMessage(c, apperror.ErrRecordNotFound, responses.MessageResourceNotFound)
	})
}
