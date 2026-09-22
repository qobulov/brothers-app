package responses

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/gofiber/fiber/v2"
)

type Meta struct {
	Timestamp  string `json:"timestamp"`
	RequestID  string `json:"request_id"`
	APIVersion string `json:"api_version"`
	Service    string `json:"service"`
	Duration   string `json:"duration"`
}

type Envelope[T any] struct {
	Success bool   `json:"success"`
	Code    int    `json:"code"`
	Slug    string `json:"slug"`
	Message string `json:"message"`
	Data    T      `json:"data"`
	Meta    Meta   `json:"meta"`
}

type requestMeta struct {
	startedAt time.Time
	requestID string
}

func Middleware(app *fiber.App) {
	app.Use(func(c *fiber.Ctx) error {
		id := c.Get("X-Request-ID")
		if !validRequestID(id) {
			id = newRequestID()
		}
		c.Set("X-Request-ID", id)
		c.Locals("request_meta", requestMeta{startedAt: time.Now().UTC(), requestID: id})
		return c.Next()
	})
}

func Success[T any](c *fiber.Ctx, status int, data T, message string) error {
	return c.Status(status).JSON(Envelope[T]{
		Success: true,
		Code:    0,
		Slug:    "ok",
		Message: message,
		Data:    data,
		Meta:    meta(c),
	})
}

func Failure(c *fiber.Ctx, status, code int, slug, message string, data any) error {
	return c.Status(status).JSON(Envelope[any]{
		Success: false,
		Code:    code,
		Slug:    slug,
		Message: message,
		Data:    data,
		Meta:    meta(c),
	})
}

func meta(c *fiber.Ctx) Meta {
	value, ok := c.Locals("request_meta").(requestMeta)
	if !ok {
		value = requestMeta{startedAt: time.Now().UTC(), requestID: newRequestID()}
	}
	return Meta{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		RequestID:  value.requestID,
		APIVersion: "v1",
		Service:    "brothers_app",
		Duration:   time.Since(value.startedAt).String(),
	}
}

func newRequestID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "request-unknown"
	}
	return hex.EncodeToString(buffer)
}

func validRequestID(value string) bool {
	if len(value) < 8 || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && char != '-' && char != '_' {
			return false
		}
	}
	return true
}
