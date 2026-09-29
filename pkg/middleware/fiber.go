package middleware

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"

	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/config"
	"github.com/qobulov/brothers-app/pkg/responses"
)

// LoadCommon sets common global middleware for the app
func FiberMiddleware(app *fiber.App, cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("CORS config is nil")
	}
	allowOrigins := strings.TrimSpace(cfg.CORSAllowOrigins)
	if allowOrigins == "" {
		allowOrigins = "*"
	}
	if cfg.CORSAllowCredentials && allowOrigins == "*" {
		return fmt.Errorf("CORS_ALLOW_CREDENTIALS cannot be true when CORS_ALLOW_ORIGINS is wildcard")
	}

	app.Use(
		logger.New(), // Logs all requests
		cors.New(cors.Config{
			AllowOrigins:     allowOrigins,
			AllowMethods:     "GET,POST,PUT,PATCH,DELETE,HEAD,OPTIONS",
			AllowHeaders:     "Origin,Content-Type,Accept,Authorization",
			AllowCredentials: cfg.CORSAllowCredentials,
			MaxAge:           86400,
		}),
	)
	return nil
}

// RequestTimeout propagates one deadline through the complete request path.
// Database and Redis operations receive this context through c.UserContext().
func RequestTimeout(limit time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.UserContext(), limit)
		defer cancel()
		c.SetUserContext(ctx)

		err := c.Next()
		if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return err
		}
		if c.Response().StatusCode() >= fiber.StatusBadRequest {
			return err
		}
		return responses.Error(c, apperror.ErrTimeout)
	}
}
