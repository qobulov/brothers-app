package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	authHandler "github.com/qobulov/brothers-app/internal/auth"
	"github.com/qobulov/brothers-app/internal/auth/otp"
	authService "github.com/qobulov/brothers-app/internal/auth/service"
	"github.com/qobulov/brothers-app/internal/auth/session"
	"github.com/qobulov/brothers-app/pkg/config"
	"github.com/qobulov/brothers-app/pkg/middleware"
)

func RegisterPublicRoutes(app fiber.Router, pool *pgxpool.Pool, otpCache *otp.Cache, sessions session.Store, cfg *config.Config, emailSender authService.EmailSender) {

	api := app.Group("/api/v1")

	authService := authService.New(pool, otpCache, sessions, cfg, emailSender)
	authHandler := authHandler.NewHandler(authService)

	// === Public Routes ===

	// Auth routes (separated from /users)
	authGroup := api.Group("/auth")
	authGroup.Post("/register", authHandler.Register)
	// A token is optional here; the email_change purpose requires one.
	authGroup.Post("/otp/send", middleware.OptionalSessionJWTMiddleware(sessions, cfg), authHandler.SendOTP)
	authGroup.Get("/username/check", authHandler.CheckUsername)
	authGroup.Post("/login", authHandler.Login)
	authGroup.Post("/refresh", authHandler.Refresh)
	authGroup.Post("/password/verify", authHandler.VerifyPassword)
	authGroup.Post("/password/reset", authHandler.ResetPassword)
}
