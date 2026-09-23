package routes

import (
	"github.com/gofiber/contrib/swagger"
	"github.com/gofiber/fiber/v2"

	docs "github.com/qobulov/brothers-app/docs/v1"
)

// SwaggerRoute func for describe group of API Docs routes.
func SwaggerRoute(a *fiber.App) {
	a.Use(swagger.New(swagger.Config{
		BasePath:    "/api/v1/",
		FilePath:    "swagger.json",
		FileContent: []byte(docs.SwaggerInfo.ReadDoc()),
		Path:        "docs",
	}))
}
