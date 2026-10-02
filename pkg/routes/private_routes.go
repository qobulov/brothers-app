package routes

import (
	authHandler "github.com/qobulov/brothers-app/internal/auth"
	"github.com/qobulov/brothers-app/internal/auth/otp"
	authService "github.com/qobulov/brothers-app/internal/auth/service"
	"github.com/qobulov/brothers-app/internal/auth/session"
	db "github.com/qobulov/brothers-app/internal/db"
	debt "github.com/qobulov/brothers-app/internal/debt"
	group "github.com/qobulov/brothers-app/internal/group"
	notification "github.com/qobulov/brothers-app/internal/notification"
	order "github.com/qobulov/brothers-app/internal/order"
	user "github.com/qobulov/brothers-app/internal/user"
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
	userLookupHandler := user.NewHandler(user.NewService(queries))

	service := authService.New(pool, otpCache, sessions, cfg, nil)
	handler := authHandler.NewHandler(service)
	secureRoute.Get("/me", handler.CurrentUser)
	secureRoute.Patch("/me", handler.UpdateCurrentUser)
	secureRoute.Post("/me/email", handler.ChangeEmail)
	secureRoute.Post("/me/password", handler.ChangePassword)
	secureRoute.Post("/auth/logout", handler.Logout)
	secureRoute.Get("/users", userLookupHandler.Lookup)

	secureRoute.Get("/notifications", notificationHandler.List)
	secureRoute.Post("/invitations/:invitationID/action", groupHandler.Action)

	// summary is registered before /:debtID so it is not read as a debt ID.
	debtHandler := debt.NewHandler(debt.NewService(pool))
	debts := secureRoute.Group("/debts")
	debts.Get("/summary", debtHandler.Summary)
	debts.Get("/", debtHandler.List)
	debts.Post("/", debtHandler.Create)
	debts.Get("/:debtID", debtHandler.Get)
	debts.Delete("/:debtID", debtHandler.Delete)
	debts.Post("/:debtID/repayments", debtHandler.Repay)
	debts.Get("/:debtID/repayments", debtHandler.Repayments)
	debts.Post("/:debtID/complete", debtHandler.Complete)

	groups := secureRoute.Group("/groups")
	groups.Post("/", groupHandler.Create)
	groups.Get("/", groupHandler.List)
	groups.Delete("/:groupID", groupHandler.Delete)
	groups.Post("/:groupID/invitations", groupHandler.Invite)
	groups.Get("/:groupID/members", groupHandler.ListMembers)
	groups.Get("/:groupID/members/:userID", groupHandler.GetMember)
	groups.Patch("/:groupID/members/:userID", groupHandler.EditMember)
	groups.Delete("/:groupID/members/:userID", groupHandler.RemoveMember)
	groups.Post("/:groupID/members/:userID/balance-adjustments", groupHandler.AdjustBalance)
	groups.Get("/:groupID/members/:userID/balance-adjustments", groupHandler.BalanceHistory)
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
