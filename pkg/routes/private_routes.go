package routes

import (
	authHandler "github.com/qobulov/brothers-app/internal/auth"
	"github.com/qobulov/brothers-app/internal/auth/otp"
	authService "github.com/qobulov/brothers-app/internal/auth/service"
	"github.com/qobulov/brothers-app/internal/auth/telegram"
	db "github.com/qobulov/brothers-app/internal/db"
	"github.com/qobulov/brothers-app/pkg/config"
	middleware "github.com/qobulov/brothers-app/pkg/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

func RegisterPrivateRoutes(app fiber.Router, pool *pgxpool.Pool, otpCache *otp.Cache, cfg *config.Config) {

	queries := db.New(pool)
	secureRoute := app.Group("/api/v1", middleware.SessionJWTMiddleware(queries, cfg))

	telegramClient := telegram.NewClient(cfg.TelegramBotToken, cfg.TelegramBotAPIURL, cfg.TelegramHTTPTimeout, cfg.TelegramPollTimeout)
	service := authService.New(pool, otpCache, cfg, telegramClient)
	handler := authHandler.NewHandler(service)
	secureRoute.Get("/me", handler.CurrentUser)
	secureRoute.Patch("/me", handler.UpdateCurrentUser)
	secureRoute.Post("/auth/logout", handler.Logout)
	secureRoute.Post("/me/phone-change/request", handler.PhoneChangeRequest)
	secureRoute.Post("/me/phone-change/resend", handler.PhoneChangeResend)
	secureRoute.Post("/me/phone-change/confirm", handler.PhoneChangeConfirm)

}
