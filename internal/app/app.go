package app

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/qobulov/brothers-app/internal/entities"
	"github.com/qobulov/brothers-app/pkg/config"
	"github.com/qobulov/brothers-app/pkg/database"
	"github.com/qobulov/brothers-app/pkg/middleware"
	"github.com/qobulov/brothers-app/pkg/responses"
	"github.com/qobulov/brothers-app/pkg/routes"
)

// SetupRestServer configures Fiber and registers routes
func SetupRestServer(db *gorm.DB, cfg *config.Config) (*fiber.App, error) {
	app := fiber.New()
	middleware.FiberMiddleware(app)
	responses.Middleware(app)
	routes.SwaggerRoute(app)
	routes.RegisterPublicRoutes(app, db, cfg)
	routes.RegisterPrivateRoutes(app, db, cfg)
	routes.RegisterNotFoundRoute(app)
	return app, nil
}

// SetupDependencies initializes database connection and schema migrations
func SetupDependencies(env string) (*gorm.DB, *config.Config, error) {
	cfg := config.LoadConfig(env)

	db, err := database.Connect(cfg.DatabaseDSN)
	if err != nil {
		return nil, nil, err
	}

	if env == "test" {
		_ = db.Migrator().DropTable(&entities.UserSession{}, &entities.Order{}, &entities.User{})
	}
	if err := db.AutoMigrate(&entities.Order{}, &entities.User{}, &entities.UserSession{}); err != nil {
		return nil, nil, err
	}

	return db, cfg, nil
}
