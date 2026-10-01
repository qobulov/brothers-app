package responses

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/qobulov/brothers-app/pkg/localization"
)

type Meta struct {
	Timestamp  string `json:"timestamp"`
	RequestID  string `json:"request_id"`
	APIVersion string `json:"api_version"`
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

const (
	MessageRequestProcessed      = "request_processed"
	MessageGroupCreated          = "group_created"
	MessageGroupsReturned        = "groups_returned"
	MessageGroupDeleted          = "group_deleted"
	MessageInvitationCreated     = "invitation_created"
	MessageMembersReturned       = "members_returned"
	MessageMemberUpdated         = "member_updated"
	MessageUsernameAvailable     = "username_available"
	MessageUsernameTaken         = "username_taken"
	MessageMemberRemoved         = "member_removed"
	MessageBalanceAdjusted       = "balance_adjusted"
	MessageInvitationAction      = "invitation_action_processed"
	MessageUsersReturned         = "users_returned"
	MessageNotificationsReturned = "notifications_returned"
	MessageUserDeleted           = "user_deleted"
	MessageOrderCreated          = "order_created"
	MessageOrdersReturned        = "orders_returned"
	MessageOrderUpdated          = "order_updated"
	MessageOrderConfirmed        = "order_confirmed"
	MessageCancellationRequested = "order_cancellation_requested"
	MessageCancellationProcessed = "order_cancellation_processed"
	MessageResourceNotFound      = "resource_not_found"
	MessageInvalidCredentials    = "invalid_credentials"
	MessageIDRequired            = "id_required"
	MessageInvalidRequest        = "invalid_request"
	MessageInvalidID             = "invalid_id"
	MessageInvalidUsername       = "invalid_username"
)

type translation struct{ uz, ru, en string }

var messageTranslations = map[string]translation{
	MessageRequestProcessed:      {"So'rov muvaffaqiyatli bajarildi", "Запрос успешно обработан", "Request processed successfully"},
	MessageGroupCreated:          {"Guruh yaratildi", "Группа создана", "Group created"},
	MessageGroupsReturned:        {"Guruhlar olindi", "Группы получены", "Groups returned"},
	MessageGroupDeleted:          {"Guruh o'chirildi", "Группа удалена", "Group deleted"},
	MessageInvitationCreated:     {"Taklif yaratildi", "Приглашение создано", "Invitation created"},
	MessageMembersReturned:       {"A'zolar olindi", "Участники получены", "Members returned"},
	MessageMemberUpdated:         {"A'zo ma'lumotlari yangilandi", "Данные участника обновлены", "Member updated"},
	MessageUsernameAvailable:     {"Username bo'sh", "Username свободен", "Username is available"},
	MessageUsernameTaken:         {"Bu username band", "Этот username уже занят", "This username is already taken"},
	MessageMemberRemoved:         {"A'zo guruhdan chiqarildi", "Участник удалён из группы", "Member removed from group"},
	MessageBalanceAdjusted:       {"Balans yangilandi", "Баланс обновлён", "Balance updated"},
	MessageInvitationAction:      {"Taklif javobi qayta ishlandi", "Ответ на приглашение обработан", "Invitation action processed"},
	MessageUsersReturned:         {"Foydalanuvchilar olindi", "Пользователи получены", "Users returned"},
	MessageNotificationsReturned: {"Bildirishnomalar olindi", "Уведомления получены", "Notifications returned"},
	MessageUserDeleted:           {"Foydalanuvchi o'chirildi", "Пользователь удалён", "User deleted"},
	MessageOrderCreated:          {"Buyurtma yaratildi", "Заказ создан", "Order created"},
	MessageOrdersReturned:        {"Buyurtmalar olindi", "Заказы получены", "Orders returned"},
	MessageOrderUpdated:          {"Buyurtma o'zgartirildi", "Заказ изменён", "Order updated"},
	MessageOrderConfirmed:        {"Buyurtma tasdiqlandi", "Заказ подтверждён", "Order confirmed"},
	MessageCancellationRequested: {"Bekor qilish so'raldi", "Запрошена отмена заказа", "Cancellation requested"},
	MessageCancellationProcessed: {"Bekor qilish so'rovi qayta ishlandi", "Запрос на отмену обработан", "Cancellation request processed"},
	MessageResourceNotFound:      {"Resurs topilmadi", "Ресурс не найден", "Resource not found"},
	MessageInvalidCredentials:    {"Hisob ma'lumotlari noto'g'ri", "Неверные учетные данные", "Invalid credentials"},
	MessageIDRequired:            {"ID kiritilishi shart", "Необходимо указать ID", "ID is required"},
	MessageInvalidRequest:        {"So'rov ma'lumotlari noto'g'ri", "Некорректный запрос", "Invalid request"},
	MessageInvalidID:             {"ID noto'g'ri", "Некорректный ID", "Invalid ID"},
	MessageInvalidUsername:       {"Foydalanuvchi nomi noto'g'ri", "Некорректное имя пользователя", "Username is invalid"},
}

// FailureReport contains request metadata, the original error, and a bounded
// JSON request body. The body is complete in development and redacted elsewhere.
// Headers are never included.
type FailureReport struct {
	Method, Path, Environment string
	Status, Code              int
	Slug, Reason              string
	Query                     string
	RequestBody, ResponseBody string
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
		Message: localizeMessage(message, Language(c)),
		Data:    data,
		Meta:    meta(c),
	})
}

// Language is the response language for the request, from Application-Language
// or Accept-Language.
func Language(c *fiber.Ctx) string {
	return localization.RequestLanguage(c.Get(localization.HeaderApplicationLanguage), c.Get(fiber.HeaderAcceptLanguage))
}

func localizeMessage(message, language string) string {
	translation, ok := messageTranslations[message]
	if !ok {
		return message
	}
	return localized(language, translation.uz, translation.ru, translation.en)
}

func localized(language, uz, ru, en string) string {
	switch localization.ResolveAcceptLanguage(language) {
	case "uz":
		return uz
	case "ru":
		return ru
	}
	return en
}

func Failure(c *fiber.Ctx, status, code int, slug, message string, data any) error {
	reason := message
	if details, ok := data.(ErrorDetails); ok {
		reason = details.Reason
	}
	return writeFailure(c, failure{status: status, code: code, slug: slug, message: message, data: data, reason: reason})
}

// failure is one error response. reason is what the error report records,
// which may be more detailed than what the response shows.
type failure struct {
	status, code          int
	slug, message, reason string
	data                  any
}

func writeFailure(c *fiber.Ctx, f failure) error {
	status, code, slug, message, data := f.status, f.code, f.slug, f.message, f.data
	metadata := meta(c)
	response := Envelope[any]{
		Success: false,
		Code:    code,
		Slug:    slug,
		Message: localizeMessage(message, Language(c)),
		Data:    data,
		Meta:    metadata,
	}
	// Only server-side failures (database, timeouts, internal errors) are
	// reported; bad input, auth and not-found responses are expected traffic.
	if report, ok := c.Locals("error_reporter").(func(FailureReport)); ok && status >= fiber.StatusInternalServerError {
		reason := f.reason
		// Route templates exclude user-supplied path values; the query is reported separately.
		path := c.Route().Path
		if path == "" || path == "/" {
			path = "<unmatched route>"
		}
		environment, _ := c.Locals(appEnvironmentLocal).(string)
		report(FailureReport{
			Method: strings.Clone(c.Method()), Path: strings.Clone(path), Environment: environment,
			Status: status, Code: code, Slug: slug, Reason: reason,
			RequestBody: safeRequestBody(c.Body(), environment), ResponseBody: safeResponseData(data, environment), Query: safeQuery(c, environment), Meta: metadata,
		})
	}
	return c.Status(status).JSON(response)
}

const maxReportedRequestBodyBytes = 16 * 1024

func safeRequestBody(body []byte, environment string) string {
	if len(body) == 0 {
		return ""
	}
	if len(body) > maxReportedRequestBodyBytes {
		return "<request body omitted: too large>"
	}

	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return "<request body omitted: invalid JSON>"
	}
	if environment != "development" {
		redactRequestValue(value)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "<request body omitted: cannot encode>"
	}
	return string(encoded)
}

// safeQuery reports the query parameters as a JSON object, redacted like the
// request body outside development. A repeated key keeps its last value.
func safeQuery(c *fiber.Ctx, environment string) string {
	values := map[string]any{}
	c.Request().URI().QueryArgs().VisitAll(func(key, value []byte) {
		values[string(key)] = string(value)
	})
	if len(values) == 0 {
		return ""
	}
	if environment != "development" {
		redactRequestValue(values)
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return "<query omitted: cannot encode>"
	}
	return string(encoded)
}

// safeResponseData reports only the response's data object; the envelope
// fields are already in the report header.
func safeResponseData(data any, environment string) string {
	if data == nil {
		return ""
	}
	body, err := json.Marshal(data)
	if err != nil {
		return "<response data omitted: cannot encode>"
	}
	return safeRequestBody(body, environment)
}

func redactRequestValue(value any) {
	switch current := value.(type) {
	case map[string]any:
		for key, item := range current {
			if sensitiveRequestField(key) {
				current[key] = "[REDACTED]"
				continue
			}
			redactRequestValue(item)
		}
	case []any:
		for _, item := range current {
			redactRequestValue(item)
		}
	}
}

func sensitiveRequestField(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	for _, fragment := range []string{
		"password", "passwd", "pwd", "token", "secret", "authorization", "api_key", "apikey",
		"otp", "email", "phone", "username", "first_name", "last_name", "avatar_url",
	} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
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
