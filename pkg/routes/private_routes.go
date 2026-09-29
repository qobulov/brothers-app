package routes

import (
	authHandler "github.com/qobulov/brothers-app/internal/auth"
	"github.com/qobulov/brothers-app/internal/auth/otp"
	authService "github.com/qobulov/brothers-app/internal/auth/service"
	"github.com/qobulov/brothers-app/internal/auth/session"
	db "github.com/qobulov/brothers-app/internal/db"
	group "github.com/qobulov/brothers-app/internal/group"
	notification "github.com/qobulov/brothers-app/internal/notification"
	order "github.com/qobulov/brothers-app/internal/order"
	userHandler "github.com/qobulov/brothers-app/internal/user/handler/rest"
	userRepository "github.com/qobulov/brothers-app/internal/user/repository"
	userUseCase "github.com/qobulov/brothers-app/internal/user/usecase"
	"github.com/qobulov/brothers-app/pkg/config"
	middleware "github.com/qobulov/brothers-app/pkg/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

func RegisterPrivateRoutes(app fiber.Router, pool *pgxpool.Pool, otpCache *otp.Cache, sessions session.Store, cfg *config.Config) {

	queries := db.New(pool)
	secureRoute := app.Group("/api/v1", middleware.SessionJWTMiddleware(sessions, cfg))
	groupService := group.NewService(pool)
	groupHandler := group.NewHandler(groupService)
	notificationHandler := notification.NewHandler(pool)
	userLookupHandler := userHandler.NewHttpUserHandler(userUseCase.NewUserService(userRepository.NewSQLCUserRepository(queries)))

	service := authService.New(pool, otpCache, sessions, cfg, nil)
	handler := authHandler.NewHandler(service)
	secureRoute.Get("/me", handler.CurrentUser)
	secureRoute.Patch("/me", handler.UpdateCurrentUser)
	secureRoute.Post("/auth/logout", handler.Logout)
	secureRoute.Get("/users", userLookupHandler.Lookup)

	secureRoute.Get("/notifications", notificationHandler.List)
	secureRoute.Post("/invitations/:invitationID/action", groupHandler.Action)

	groups := secureRoute.Group("/groups")
	groups.Post("/", groupHandler.Create)
	groups.Get("/", groupHandler.List)
	groups.Delete("/:groupID", groupHandler.Delete)
	groups.Post("/:groupID/invitations", groupHandler.Invite)
	groups.Get("/:groupID/members", groupHandler.ListMembers)
	groups.Get("/:groupID/locations", groupHandler.ListLocations)
	groups.Post("/:groupID/locations", groupHandler.CreateLocation)
	groups.Delete("/:groupID/locations/:locationID", groupHandler.DeleteLocation)
	groups.Get("/:groupID/customers", groupHandler.ListCustomers)

	orderHandler := order.NewHandler(order.NewService(pool))
	groups.Post("/:groupID/orders", orderHandler.Create)
	groups.Get("/:groupID/orders", orderHandler.List)
	groups.Get("/:groupID/orders/:orderID", orderHandler.Get)
	groups.Patch("/:groupID/orders/:orderID", orderHandler.Edit)
	groups.Post("/:groupID/orders/:orderID/confirmations", orderHandler.Confirm)
	groups.Get("/:groupID/orders/:orderID/events", orderHandler.Events)
	groups.Post("/:groupID/orders/:orderID/cancellation", orderHandler.RequestCancellation)
	groups.Post("/:groupID/orders/:orderID/cancellation/action", orderHandler.RespondCancellation)
}
