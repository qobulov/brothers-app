package middleware

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/qobulov/brothers-app/pkg/responses"
)

type memoryCounter struct {
	hits map[string]int64
	err  error
}

func (m *memoryCounter) Increment(_ context.Context, name string, _ time.Duration) (int64, error) {
	if m.err != nil {
		return 0, m.err
	}
	m.hits[name]++
	return m.hits[name], nil
}

func limitedApp(counter Counter, limit RateLimit) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: responses.Error})
	app.Post("/login", RateLimiter(counter, limit), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	return app
}

func hit(t *testing.T, app *fiber.App, forwardedFor, body string) int {
	t.Helper()
	request := httptest.NewRequest(fiber.MethodPost, "/login", strings.NewReader(body))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	if forwardedFor != "" {
		request.Header.Set(fiber.HeaderXForwardedFor, forwardedFor)
	}
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer response.Body.Close()
	return response.StatusCode
}

func TestRateLimiter_BlocksAfterMaxPerClient(t *testing.T) {
	app := limitedApp(&memoryCounter{hits: map[string]int64{}}, RateLimit{Name: "login", Max: 2, Window: time.Minute})

	for attempt := 1; attempt <= 2; attempt++ {
		if status := hit(t, app, "203.0.113.7", ""); status != fiber.StatusOK {
			t.Fatalf("attempt %d: status %d, want 200", attempt, status)
		}
	}
	if status := hit(t, app, "203.0.113.7", ""); status != fiber.StatusTooManyRequests {
		t.Fatalf("third attempt: status %d, want 429", status)
	}
	// Behind a proxy the first forwarded address is the client.
	if status := hit(t, app, "203.0.113.8, 10.0.0.1", ""); status != fiber.StatusOK {
		t.Fatalf("another client: status %d, want 200", status)
	}
}

func TestRateLimiter_KeyNarrowsTheLimit(t *testing.T) {
	limit := RateLimit{Name: "login", Max: 1, Window: time.Minute, Key: JSONField("login")}
	app := limitedApp(&memoryCounter{hits: map[string]int64{}}, limit)

	if status := hit(t, app, "203.0.113.7", `{"login":"Abror"}`); status != fiber.StatusOK {
		t.Fatalf("first login: status %d, want 200", status)
	}
	if status := hit(t, app, "203.0.113.7", `{"login":" abror "}`); status != fiber.StatusTooManyRequests {
		t.Fatalf("same login in another case: status %d, want 429", status)
	}
	if status := hit(t, app, "203.0.113.7", `{"login":"other"}`); status != fiber.StatusOK {
		t.Fatalf("another login: status %d, want 200", status)
	}
}

func TestRateLimiter_AllowsRequestsWithoutWorkingCounter(t *testing.T) {
	limit := RateLimit{Name: "login", Max: 1, Window: time.Minute}
	broken := limitedApp(&memoryCounter{err: errors.New("redis unavailable")}, limit)
	disabled := limitedApp(nil, limit)

	for attempt := 1; attempt <= 3; attempt++ {
		if status := hit(t, broken, "203.0.113.7", ""); status != fiber.StatusOK {
			t.Fatalf("broken counter, attempt %d: status %d, want 200", attempt, status)
		}
		if status := hit(t, disabled, "203.0.113.7", ""); status != fiber.StatusOK {
			t.Fatalf("no counter, attempt %d: status %d, want 200", attempt, status)
		}
	}
}
