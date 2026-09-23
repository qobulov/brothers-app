package middleware

import (
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/qobulov/brothers-app/internal/db"
	"github.com/qobulov/brothers-app/pkg/config"
	"github.com/qobulov/brothers-app/pkg/responses"
)

func JWTMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		auth := c.Get("Authorization")
		if auth == "" {
			return unauthorized(c)
		}
		parts := strings.Fields(auth)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return unauthorized(c)
		}
		tokenStr := parts[1]

		// tokenStr := c.Cookies("token") // Assuming the token is stored in a cookie named "token"
		// if tokenStr == "" {
		// 	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing token"})
		// }

		token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(os.Getenv("JWT_SECRET")), nil
		}, jwt.WithValidMethods([]string{"HS256"}))

		if err != nil || !token.Valid {
			return unauthorized(c)
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return unauthorized(c)
		}
		userID := claims["user_id"]
		c.Locals("user_id", userID)

		return c.Next()
	}
}

// SessionJWTMiddleware validates an access token and checks that its single
// referenced session is still active. It is used by the new auth endpoints.
func SessionJWTMiddleware(queries *db.Queries, cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		parts := strings.Fields(c.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return unauthorized(c)
		}
		token, err := jwt.Parse(parts[1], func(token *jwt.Token) (interface{}, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(cfg.JWTSecret), nil
		}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(cfg.JWTIssuer), jwt.WithAudience(cfg.JWTAudience), jwt.WithIssuedAt())
		if err != nil || !token.Valid {
			return unauthorized(c)
		}
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok || claims["type"] != "access" {
			return unauthorized(c)
		}
		sub, okSub := claims["sub"].(string)
		sid, okSID := claims["sid"].(string)
		userID, userErr := uuid.Parse(sub)
		sessionID, sessionErr := uuid.Parse(sid)
		if !okSub || !okSID || userErr != nil || sessionErr != nil {
			return unauthorized(c)
		}
		_, err = queries.GetActiveSession(c.UserContext(), db.GetActiveSessionParams{
			ID:        pgtype.UUID{Bytes: sessionID, Valid: true},
			UserID:    pgtype.UUID{Bytes: userID, Valid: true},
			ExpiresAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		})
		if err != nil {
			return unauthorized(c)
		}
		c.Locals("auth_user_id", userID)
		c.Locals("auth_session_id", sessionID)
		return c.Next()
	}
}

func unauthorized(c *fiber.Ctx) error {
	return responses.Failure(c, fiber.StatusUnauthorized, 1401, "unauthorized", "Неверные учетные данные", nil)
}
