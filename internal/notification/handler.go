package notification

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/localization"
	"github.com/qobulov/brothers-app/pkg/responses"
)

type Handler struct {
	pool *pgxpool.Pool
}

type Notification struct {
	ID        uuid.UUID      `json:"id"`
	Title     string         `json:"title"`
	Content   string         `json:"content"`
	Type      string         `json:"type" enums:"GLOBAL,TARGETED"`
	EventType string         `json:"event_type" enums:"GROUP_INVITATION,ORDER_CREATED,ORDER_CONFIRMED,MEMBER_JOINED,ORDER_CANCELLED"`
	Payload   map[string]any `json:"payload" swaggertype:"object"`
	IsRead    bool           `json:"is_read"`
	ReadAt    *time.Time     `json:"read_at,omitempty"`
	ExpiresAt *time.Time     `json:"expires_at,omitempty"`
}

type NotificationsResponse struct {
	Success bool           `json:"success"`
	Code    int            `json:"code"`
	Slug    string         `json:"slug"`
	Message string         `json:"message"`
	Data    []Notification `json:"data"`
	Meta    responses.Meta `json:"meta"`
}

func NewHandler(pool *pgxpool.Pool) *Handler {
	return &Handler{pool: pool}
}

// List godoc
// @Summary List my notifications
// @Tags notifications
// @Produce json
// @Success 200 {object} NotificationsResponse
// @Failure 401 {object} group.ErrorResponse
// @Security BearerAuth
// @Router /notifications [get]
func (h *Handler) List(c *fiber.Ctx) error {
	userID, err := userID(c)
	if err != nil {
		return responses.Error(c, err)
	}
	language := localization.ResolveAcceptLanguage(c.Get(fiber.HeaderAcceptLanguage))
	rows, err := h.pool.Query(c.UserContext(), `
		SELECT notifications.id,
		       CASE $2 WHEN 'uz' THEN notifications.title_uz WHEN 'ru' THEN notifications.title_ru ELSE notifications.title_en END,
		       CASE $2 WHEN 'uz' THEN notifications.content_uz WHEN 'ru' THEN notifications.content_ru ELSE notifications.content_en END,
		       notifications.type::text,
		       notifications.payload, recipients.is_read,
		       recipients.read_at, notifications.expires_at
		FROM notification_recipients recipients
		JOIN notifications ON notifications.id = recipients.notification_id
		WHERE recipients.user_id = $1
		  AND recipients.deleted_at IS NULL
		  AND notifications.deleted_at IS NULL
		  AND (notifications.expires_at IS NULL OR notifications.expires_at > now())
		ORDER BY notifications.created_at DESC
	`, userID, language)
	if err != nil {
		return responses.Error(c, fmt.Errorf("listing notifications: %w", err))
	}
	defer rows.Close()
	items := make([]Notification, 0)
	for rows.Next() {
		var item Notification
		var payload json.RawMessage
		if err := rows.Scan(&item.ID, &item.Title, &item.Content, &item.Type, &payload, &item.IsRead, &item.ReadAt, &item.ExpiresAt); err != nil {
			return responses.Error(c, fmt.Errorf("scanning notification: %w", err))
		}
		if err := json.Unmarshal(payload, &item.Payload); err != nil {
			return responses.Error(c, fmt.Errorf("decoding notification payload object: %w", err))
		}
		eventType, ok := item.Payload["event_type"].(string)
		if !ok || eventType == "" {
			return responses.Error(c, fmt.Errorf("notification payload event_type is missing"))
		}
		item.EventType = eventType
		delete(item.Payload, "event_type")
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return responses.Error(c, fmt.Errorf("iterating notifications: %w", err))
	}
	return responses.Success(c, fiber.StatusOK, items, responses.MessageNotificationsReturned)
}

func userID(c *fiber.Ctx) (uuid.UUID, error) {
	id, ok := c.Locals("auth_user_id").(uuid.UUID)
	if !ok || id == uuid.Nil {
		return uuid.Nil, apperror.ErrUnauthorized
	}
	return id, nil
}
