package app

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/jackc/pgx/v5/pgxpool"

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
	app := fiber.New(fiber.Config{ErrorHandler: responses.Error})
	if err := middleware.FiberMiddleware(app, cfg); err != nil {
		return nil, err
	}
	reporter := telegramlog.New(cfg)
	responses.Middleware(app, cfg.AppEnv, reporter.Report)
	app.Use(recover.New())
	app.Hooks().OnShutdown(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return reporter.Close(ctx)
	})
	routes.SwaggerRoute(app)
	routes.RegisterPublicRoutes(app, pool, otpCache, sessions, cfg)
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
