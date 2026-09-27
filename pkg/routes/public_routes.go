package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	authHandler "github.com/qobulov/brothers-app/internal/auth"
	"github.com/qobulov/brothers-app/internal/auth/email"
	"github.com/qobulov/brothers-app/internal/auth/otp"
	authService "github.com/qobulov/brothers-app/internal/auth/service"
	"github.com/qobulov/brothers-app/internal/auth/session"
	db "github.com/qobulov/brothers-app/internal/db"
	"github.com/qobulov/brothers-app/pkg/config"

	// Order
	orderHandler "github.com/qobulov/brothers-app/internal/order/handler/rest"
	orderRepository "github.com/qobulov/brothers-app/internal/order/repository"
	orderUseCase "github.com/qobulov/brothers-app/internal/order/usecase"
)

func RegisterPublicRoutes(app fiber.Router, pool *pgxpool.Pool, otpCache *otp.Cache, sessions session.Store, cfg *config.Config) {

	api := app.Group("/api/v1")

	// === Dependency Wiring ===

	// Order
	queries := db.New(pool)
	orderRepo := orderRepository.NewSQLCOrderRepository(queries)
	orderService := orderUseCase.NewOrderService(orderRepo)
	orderHandler := orderHandler.NewHttpOrderHandler(orderService)

	emailClient := email.New(cfg)
	authService := authService.New(pool, otpCache, sessions, cfg, emailClient)
	authHandler := authHandler.NewHandler(authService)

	// === Public Routes ===

	// Auth routes (separated from /users)
	authGroup := api.Group("/auth")
	authGroup.Post("/register", authHandler.Register)
	authGroup.Post("/otp/send", authHandler.SendOTP)
	authGroup.Post("/login", authHandler.Login)
	authGroup.Post("/refresh", authHandler.Refresh)
	authGroup.Post("/password/verify", authHandler.VerifyPassword)
	authGroup.Post("/password/reset", authHandler.ResetPassword)

	// Order routes
	orderGroup := api.Group("/orders")
	orderGroup.Get("/", orderHandler.FindAllOrders)
	orderGroup.Get("/:id", orderHandler.FindOrderByID)
	orderGroup.Post("/", orderHandler.CreateOrder)
	orderGroup.Patch("/:id", orderHandler.PatchOrder)
	orderGroup.Delete("/:id", orderHandler.DeleteOrder)
}
