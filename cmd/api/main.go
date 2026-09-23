package main

import (
	_ "github.com/qobulov/brothers-app/docs/v1"
	"github.com/qobulov/brothers-app/internal/app"
)

// @title Brothers App API
// @version 1.0
// @description REST API for Brothers App following Clean Architecture.
// @BasePath /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.
func main() {
	app.Start()
}
