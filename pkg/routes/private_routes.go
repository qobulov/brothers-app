package routes

import (
	authHandler "github.com/qobulov/brothers-app/internal/auth"
	authService "github.com/qobulov/brothers-app/internal/auth/service"
	"github.com/qobulov/brothers-app/internal/auth/telegram"
	"github.com/qobulov/brothers-app/pkg/config"
	middleware "github.com/qobulov/brothers-app/pkg/middleware"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func RegisterPrivateRoutes(app fiber.Router, db *gorm.DB, cfg *config.Config) {

	secureRoute := app.Group("/api/v1", middleware.SessionJWTMiddleware(db, cfg))

	telegramClient := telegram.NewClient(cfg.TelegramBotToken, cfg.TelegramBotAPIURL, cfg.TelegramHTTPTimeout, cfg.TelegramPollTimeout)
	service := authService.New(db, cfg, telegramClient)
	handler := authHandler.NewHandler(service)
	secureRoute.Get("/me", handler.CurrentUser)
	secureRoute.Post("/auth/logout", handler.Logout)
	secureRoute.Post("/me/phone-change/request", handler.PhoneChangeRequest)
	secureRoute.Post("/me/phone-change/resend", handler.PhoneChangeResend)
	secureRoute.Post("/me/phone-change/confirm", handler.PhoneChangeConfirm)

}
