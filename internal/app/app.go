package app

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qobulov/brothers-app/internal/auth/otp"
	"github.com/qobulov/brothers-app/pkg/config"
	"github.com/qobulov/brothers-app/pkg/database"
	"github.com/qobulov/brothers-app/pkg/middleware"
	"github.com/qobulov/brothers-app/pkg/responses"
	"github.com/qobulov/brothers-app/pkg/routes"
)

// SetupRestServer configures Fiber and registers routes
func SetupRestServer(pool *pgxpool.Pool, otpCache *otp.Cache, cfg *config.Config) (*fiber.App, error) {
	app := fiber.New()
	middleware.FiberMiddleware(app)
	responses.Middleware(app)
	routes.SwaggerRoute(app)
	routes.RegisterPublicRoutes(app, pool, otpCache, cfg)
	routes.RegisterPrivateRoutes(app, pool, otpCache, cfg)
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
