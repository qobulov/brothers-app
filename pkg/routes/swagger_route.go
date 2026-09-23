package routes

import (
	"github.com/gofiber/contrib/swagger"
	"github.com/gofiber/fiber/v2"

	docs "github.com/qobulov/brothers-app/docs/v1"
)

// SwaggerRoute func for describe group of API Docs routes.
func SwaggerRoute(a *fiber.App) {
	// Resolve API requests against the origin serving Swagger so the same build
	// works locally and behind Vercel's HTTPS reverse proxy.
	docs.SwaggerInfo.Host = ""
	docs.SwaggerInfo.Schemes = nil

	a.Use(swagger.New(swagger.Config{
		BasePath:    "/api/v1/",
		FilePath:    "swagger.json",
		FileContent: []byte(docs.SwaggerInfo.ReadDoc()),
		Path:        "docs",
	}))

	a.Get("/", func(c *fiber.Ctx) error {
		return c.Redirect("/api/v1/docs", fiber.StatusTemporaryRedirect)
	})
}
