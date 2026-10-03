package middleware

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/responses"
)

// Counter counts hits of a name inside a time window, shared by all app instances.
type Counter interface {
	Increment(ctx context.Context, name string, window time.Duration) (int64, error)
}

// RateLimit allows Max requests per Window for one client.
type RateLimit struct {
	Name   string
	Max    int64
	Window time.Duration
	// Key narrows the limit from the client to one value, e.g. one login name.
	Key func(c *fiber.Ctx) string
}

// RateLimiter rejects a client that exceeds the limit with 429. Without a
// counter, or when the counter fails, requests pass: Redis being down must not
// lock everyone out.
func RateLimiter(counter Counter, limit RateLimit) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if counter == nil {
			return c.Next()
		}
		count, err := counter.Increment(c.UserContext(), counterName(c, limit), limit.Window)
		if err != nil {
			slog.Warn("counting rate limit", "limit", limit.Name, "error", err)
			return c.Next()
		}
		if count > limit.Max {
			return responses.Error(c, tooManyRequests())
		}
		return c.Next()
	}
}

func counterName(c *fiber.Ctx, limit RateLimit) string {
	name := "rate:" + limit.Name + ":" + clientIP(c)
	if limit.Key != nil {
		name += ":" + limit.Key(c)
	}
	return name
}

// clientIP prefers the address reported by the hosting proxy: behind it every
// connection comes from the proxy itself, and one client would block everyone.
func clientIP(c *fiber.Ctx) string {
	forwarded, _, _ := strings.Cut(c.Get(fiber.HeaderXForwardedFor), ",")
	if forwarded = strings.TrimSpace(forwarded); forwarded != "" {
		return forwarded
	}
	return c.IP()
}

// JSONField keys a limit by one string field of the JSON body, ignoring case.
func JSONField(field string) func(c *fiber.Ctx) string {
	return func(c *fiber.Ctx) string {
		var body map[string]any
		if err := c.BodyParser(&body); err != nil {
			return ""
		}
		value, _ := body[field].(string)
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func tooManyRequests() error {
	return apperror.New(apperror.ErrLimitExceeded, apperror.Text{
		UZ: "So'rovlar juda ko'p. Birozdan keyin qayta urinib ko'ring",
		RU: "Слишком много запросов. Повторите попытку позже",
		EN: "Too many requests. Please try again later",
	})
}
