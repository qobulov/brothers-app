package notification

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qobulov/brothers-app/pkg/request"
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
	EventType string         `json:"event_type" enums:"GROUP_INVITATION,ORDER_CREATED,ORDER_UPDATED,ORDER_AMOUNT_MISMATCH,ORDER_COMPLETED,ORDER_CANCELLATION_REQUESTED,ORDER_CANCELLATION_REJECTED,ORDER_CANCELLED"`
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
// @Param limit query int false "Page size" default(50) minimum(1) maximum(100)
// @Param offset query int false "Number of notifications to skip" default(0) minimum(0)
// @Success 200 {object} NotificationsResponse
// @Failure 400 {object} group.ErrorResponse
// @Failure 401 {object} group.ErrorResponse
// @Security BearerAuth
// @Router /notifications [get]
func (h *Handler) List(c *fiber.Ctx) error {
	userID, err := request.UserID(c)
	if err != nil {
		return responses.Error(c, err)
	}
	page, err := request.Page(c)
	if err == nil {
		page, err = page.Valid()
	}
	if err != nil {
		return responses.Error(c, err)
	}
	language := responses.Language(c)
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
		ORDER BY recipients.created_at DESC, recipients.id DESC
		LIMIT $3 OFFSET $4
	`, userID, language, page.Limit, page.Offset)
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
		// One malformed row must not hide every other notification from the user.
		if err := json.Unmarshal(payload, &item.Payload); err != nil {
			slog.Warn("skipping notification with invalid payload", "notification_id", item.ID, "error", err)
			continue
		}
		eventType, ok := item.Payload["event_type"].(string)
		if !ok || eventType == "" {
			slog.Warn("skipping notification without event_type", "notification_id", item.ID)
			continue
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
