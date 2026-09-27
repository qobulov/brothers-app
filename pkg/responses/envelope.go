package responses

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
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

const appEnvironmentLocal = "app_environment"

// FailureReport contains request metadata and the original error, never the body or headers.
type FailureReport struct {
	Method, Path, Environment string
	Status, Code              int
	Slug, Reason              string
	Meta                      Meta
}

func Middleware(app *fiber.App, appEnvironment string, reporters ...func(FailureReport)) {
	appEnvironment = strings.ToLower(strings.TrimSpace(appEnvironment))

	app.Use(func(c *fiber.Ctx) error {
		id := c.Get("X-Request-ID")
		if !validRequestID(id) {
			id = newRequestID()
		}
		c.Set("X-Request-ID", id)
		c.Locals("request_meta", requestMeta{startedAt: time.Now().UTC(), requestID: id})
		c.Locals(appEnvironmentLocal, appEnvironment)
		if len(reporters) > 0 && reporters[0] != nil {
			c.Locals("error_reporter", reporters[0])
		}
		return c.Next()
	})
}

func Success[T any](c *fiber.Ctx, status int, data T, message string) error {
	return c.Status(status).JSON(Envelope[T]{
		Success: true,
		Code:    0,
		Slug:    "ok",
		Message: localizeMessage(message, c.Get(fiber.HeaderAcceptLanguage)),
		Data:    data,
		Meta:    meta(c),
	})
}

func localizeMessage(message, language string) string {
	switch message {
	case "Request processed successfully", "Запрос успешно обработан":
		return localized(language, "So'rov muvaffaqiyatli bajarildi", "Запрос успешно обработан", "Request processed successfully")
	case "user deleted":
		return localized(language, "Foydalanuvchi o'chirildi", "Пользователь удалён", "User deleted")
	case "order deleted":
		return localized(language, "Buyurtma o'chirildi", "Заказ удалён", "Order deleted")
	case "Resurs topilmadi", "Ресурс не найден", "Resource not found":
		return localized(language, "Resurs topilmadi", "Ресурс не найден", "Resource not found")
	case "Hisob ma'lumotlari noto'g'ri", "Неверные учетные данные", "Invalid credentials":
		return localized(language, "Hisob ma'lumotlari noto'g'ri", "Неверные учетные данные", "Invalid credentials")
	case "id is required":
		return localized(language, "ID kiritilishi shart", "Необходимо указать ID", "ID is required")
	case "invalid request":
		return localized(language, "So'rov ma'lumotlari noto'g'ri", "Некорректный запрос", "Invalid request")
	case "invalid id":
		return localized(language, "ID noto'g'ri", "Некорректный ID", "Invalid ID")
	case "username is invalid":
		return localized(language, "Foydalanuvchi nomi noto'g'ri", "Некорректное имя пользователя", "Username is invalid")
	case "total must be positive":
		return localized(language, "Umumiy summa musbat bo'lishi kerak", "Сумма должна быть положительной", "Total must be positive")
	default:
		return message
	}
}

func localized(language, uz, ru, en string) string {
	for _, candidate := range strings.Split(strings.ToLower(language), ",") {
		candidate = strings.TrimSpace(strings.SplitN(candidate, ";", 2)[0])
		candidate = strings.SplitN(candidate, "-", 2)[0]
		switch candidate {
		case "uz":
			return uz
		case "ru":
			return ru
		case "en":
			return en
		}
	}
	return en
}

func Failure(c *fiber.Ctx, status, code int, slug, message string, data any) error {
	metadata := meta(c)
	if report, ok := c.Locals("error_reporter").(func(FailureReport)); ok {
		reason := message
		if details, ok := data.(ErrorDetails); ok {
			reason = details.Reason
		}
		// Route templates exclude query parameters and user-supplied path values.
		path := c.Route().Path
		if path == "" || path == "/" {
			path = "<unmatched route>"
		}
		environment, _ := c.Locals(appEnvironmentLocal).(string)
		report(FailureReport{
			Method: strings.Clone(c.Method()), Path: strings.Clone(path), Environment: environment,
			Status: status, Code: code, Slug: slug, Reason: reason, Meta: metadata,
		})
	}
	return c.Status(status).JSON(Envelope[any]{
		Success: false,
		Code:    code,
		Slug:    slug,
		Message: localizeMessage(message, c.Get(fiber.HeaderAcceptLanguage)),
		Data:    data,
		Meta:    metadata,
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
