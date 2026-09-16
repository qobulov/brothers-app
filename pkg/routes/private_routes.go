package routes

import (
	userHandler "github.com/qobulov/brothers-app/internal/user/handler/rest"
	userRepository "github.com/qobulov/brothers-app/internal/user/repository"
	userUseCase "github.com/qobulov/brothers-app/internal/user/usecase"
	middleware "github.com/qobulov/brothers-app/pkg/middleware"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func RegisterPrivateRoutes(app fiber.Router, db *gorm.DB) {

	route := app.Group("/api/v1", middleware.JWTMiddleware())

	userRepo := userRepository.NewGormUserRepository(db)
	userService := userUseCase.NewUserService(userRepo)
	userHandler := userHandler.NewHttpUserHandler(userService)

	route.Get("/me", userHandler.GetUser)

}
