package routes

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	authHandler "github.com/qobulov/brothers-app/internal/auth"
	"github.com/qobulov/brothers-app/internal/auth/otp"
	authService "github.com/qobulov/brothers-app/internal/auth/service"
	"github.com/qobulov/brothers-app/internal/auth/session"
	cachepkg "github.com/qobulov/brothers-app/pkg/cache"
	"github.com/qobulov/brothers-app/pkg/config"
	"github.com/qobulov/brothers-app/pkg/middleware"
)

func RegisterPublicRoutes(app fiber.Router, pool *pgxpool.Pool, otpCache *otp.Cache, sessions session.Store, cfg *config.Config, emailSender authService.EmailSender, responseCache *cachepkg.ResponseCache) {

	api := app.Group("/api/v1", responseCache.Cache())

	authService := authService.New(pool, otpCache, sessions, cfg, emailSender)
	authHandler := authHandler.NewHandler(authService)

	// === Public Routes ===

	// Auth routes (separated from /users)
	authGroup := api.Group("/auth")
	limit := func(rule middleware.RateLimit) fiber.Handler {
		return middleware.RateLimiter(rateCounter(otpCache), rule)
	}
	authGroup.Post("/register", responseCache.Invalidate(), authHandler.Register)
	// A token is optional here; the email_change purpose requires one.
	authGroup.Post("/otp/send", limit(otpSendLimit), middleware.OptionalSessionJWTMiddleware(sessions, cfg), authHandler.SendOTP)
	authGroup.Get("/username/check", limit(usernameCheckLimit), authHandler.CheckUsername)
	authGroup.Post("/login", limit(loginLimit), limit(loginPerAccountLimit), responseCache.Invalidate(), authHandler.Login)
	authGroup.Post("/refresh", authHandler.Refresh)
	authGroup.Post("/password/verify", authHandler.VerifyPassword)
	authGroup.Post("/password/reset", responseCache.Invalidate(), authHandler.ResetPassword)
}

// Limits are per client IP. Mobile carriers put many users behind one address,
// so the per-IP numbers are generous; guessing one account's password is
// limited much harder.
var (
	loginLimit           = middleware.RateLimit{Name: "login", Max: 100, Window: 15 * time.Minute}
	loginPerAccountLimit = middleware.RateLimit{Name: "login-account", Max: 10, Window: 15 * time.Minute, Key: middleware.JSONField("login")}
	usernameCheckLimit   = middleware.RateLimit{Name: "username-check", Max: 60, Window: time.Minute}
	otpSendLimit         = middleware.RateLimit{Name: "otp-send", Max: 20, Window: 15 * time.Minute}
)

// rateCounter returns nil when Redis is not wired, which disables the limits.
func rateCounter(otpCache *otp.Cache) middleware.Counter {
	if otpCache == nil {
		return nil
	}
	return otpCache
}
