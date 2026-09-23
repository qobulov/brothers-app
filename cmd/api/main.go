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
// @description Paste the access token only. Standard clients may send "Bearer <token>"; Swagger UI sends the token value directly.
func main() {
	app.Start()
}
