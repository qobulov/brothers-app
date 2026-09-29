package app

import (
	"context"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qobulov/brothers-app/internal/auth/email"
	"github.com/qobulov/brothers-app/internal/auth/otp"
	"github.com/qobulov/brothers-app/internal/auth/session"
	"github.com/qobulov/brothers-app/pkg/config"
	"github.com/qobulov/brothers-app/pkg/database"
	"github.com/qobulov/brothers-app/pkg/middleware"
	"github.com/qobulov/brothers-app/pkg/responses"
	"github.com/qobulov/brothers-app/pkg/routes"
	"github.com/qobulov/brothers-app/pkg/telegramlog"
)

// SetupRestServer configures Fiber and registers routes
func SetupRestServer(pool *pgxpool.Pool, otpCache *otp.Cache, sessions session.Store, cfg *config.Config) (*fiber.App, error) {
	requestTimeout := cfg.APIRequestTimeout
	if requestTimeout <= 0 {
		requestTimeout = 5 * time.Second
	}
	app := fiber.New(fiber.Config{
		ErrorHandler: responses.Error,
		ReadTimeout:  requestTimeout,
		WriteTimeout: requestTimeout + time.Second,
		IdleTimeout:  60 * time.Second,
	})
	if err := middleware.FiberMiddleware(app, cfg); err != nil {
		return nil, err
	}
	reporter := telegramlog.New(cfg)
	emailDispatcher := email.NewDispatcher(email.New(cfg), 2, 64)
	responses.Middleware(app, cfg.AppEnv, reporter.Report)
	app.Use(recover.New())
	app.Use(middleware.RequestTimeout(requestTimeout))
	app.Hooks().OnShutdown(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return errors.Join(emailDispatcher.Close(ctx), reporter.Close(ctx))
	})
	routes.SwaggerRoute(app)
	routes.RegisterPublicRoutes(app, pool, otpCache, sessions, cfg, emailDispatcher)
	routes.RegisterPrivateRoutes(app, pool, otpCache, sessions, cfg)
	routes.RegisterNotFoundRoute(app)
	return app, nil
}

// SetupDependencies initializes application configuration and the database pool.
func SetupDependencies(env string) (*pgxpool.Pool, *config.Config, error) {
	cfg := config.LoadConfig(env)

	pool, err := database.Connect(context.Background(), cfg.DatabaseDSN)
	if err != nil {
		return nil, nil, err
	}

	return pool, cfg, nil
}
