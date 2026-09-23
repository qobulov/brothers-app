package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	authHandler "github.com/qobulov/brothers-app/internal/auth"
	"github.com/qobulov/brothers-app/internal/auth/otp"
	authService "github.com/qobulov/brothers-app/internal/auth/service"
	"github.com/qobulov/brothers-app/internal/auth/telegram"
	db "github.com/qobulov/brothers-app/internal/db"
	"github.com/qobulov/brothers-app/pkg/config"

	// Order
	orderHandler "github.com/qobulov/brothers-app/internal/order/handler/rest"
	orderRepository "github.com/qobulov/brothers-app/internal/order/repository"
	orderUseCase "github.com/qobulov/brothers-app/internal/order/usecase"

	// User
	userHandler "github.com/qobulov/brothers-app/internal/user/handler/rest"
	userRepository "github.com/qobulov/brothers-app/internal/user/repository"
	userUseCase "github.com/qobulov/brothers-app/internal/user/usecase"
)

func RegisterPublicRoutes(app fiber.Router, pool *pgxpool.Pool, otpCache *otp.Cache, cfg *config.Config) {

	api := app.Group("/api/v1")

	// === Dependency Wiring ===

	// Order
	queries := db.New(pool)
	orderRepo := orderRepository.NewSQLCOrderRepository(queries)
	orderService := orderUseCase.NewOrderService(orderRepo)
	orderHandler := orderHandler.NewHttpOrderHandler(orderService)

	// User
	userRepo := userRepository.NewSQLCUserRepository(queries)
	userService := userUseCase.NewUserService(userRepo)
	userHandler := userHandler.NewHttpUserHandler(userService)
	telegramClient := telegram.NewClient(cfg.TelegramBotToken, cfg.TelegramBotAPIURL, cfg.TelegramHTTPTimeout, cfg.TelegramPollTimeout)
	authService := authService.New(pool, otpCache, cfg, telegramClient)
	authHandler := authHandler.NewHandler(authService)

	// === Public Routes ===

	// Auth routes (separated from /users)
	authGroup := api.Group("/auth")
	authGroup.Post("/signup", userHandler.Register)
	authGroup.Post("/signin", userHandler.Login)
	authGroup.Post("/register", authHandler.Register)
	authGroup.Post("/login", authHandler.Login)
	authGroup.Post("/refresh", authHandler.Refresh)
	authGroup.Post("/otp/verify", authHandler.VerifyRegistration)
	authGroup.Post("/register/resend", authHandler.ResendRegistration)
	authGroup.Post("/password/forgot", authHandler.ForgotPassword)
	authGroup.Post("/password/resend", authHandler.ResendPassword)
	authGroup.Post("/password/verify", authHandler.VerifyPassword)
	authGroup.Post("/password/reset", authHandler.ResetPassword)

	// User routes
	userGroup := api.Group("/users")
	userGroup.Get("/", userHandler.FindAllUsers)
	userGroup.Get("/:id", userHandler.FindUserByID)
	userGroup.Patch("/:id", userHandler.PatchUser)
	userGroup.Delete("/:id", userHandler.DeleteUser)

	// Order routes
	orderGroup := api.Group("/orders")
	orderGroup.Get("/", orderHandler.FindAllOrders)
	orderGroup.Get("/:id", orderHandler.FindOrderByID)
	orderGroup.Post("/", orderHandler.CreateOrder)
	orderGroup.Patch("/:id", orderHandler.PatchOrder)
	orderGroup.Delete("/:id", orderHandler.DeleteOrder)
}
