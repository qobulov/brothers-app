package notification

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/database"
	"github.com/qobulov/brothers-app/pkg/responses"
)

func TestHandler_ListLocalizesContentAndKeepsSlug(t *testing.T) {
	pool, cleanup := database.SetupTestDB(t)
	defer cleanup()

	userID := uuid.New()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO users (id, email, username, language, is_active, created_at, updated_at)
		VALUES ($1, 'notification@example.com', 'notification-user', 'en', true, now(), now())
	`, userID)
	if err != nil {
		t.Fatalf("create notification user: %v", err)
	}

	var notificationID uuid.UUID
	err = pool.QueryRow(context.Background(), `
		INSERT INTO notifications (
			title_en, title_uz, title_ru, content_en, content_uz, content_ru,
			type, payload, created_at
		)
		VALUES (
			'Group invitation', 'Guruhga taklif', 'Приглашение в группу',
			'You have been invited to join Oilam',
			'Siz Oilam guruhiga qo''shilish uchun taklif qilindingiz',
			'Вас пригласили присоединиться к группе Oilam',
			'TARGETED'::notification_type,
			'{"event_type":"GROUP_INVITATION","group_name":"Oilam"}'::jsonb,
			now()
		)
		RETURNING id
	`).Scan(&notificationID)
	if err != nil {
		t.Fatalf("create localized notification: %v", err)
	}
	_, err = pool.Exec(context.Background(), `
		INSERT INTO notification_recipients (notification_id, user_id, is_read)
		VALUES ($1, $2, false)
	`, notificationID, userID)
	if err != nil {
		t.Fatalf("create notification recipient: %v", err)
	}

	app := fiber.New()
	responses.Middleware(app, "test")
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("auth_user_id", userID)
		return c.Next()
	})
	app.Get("/notifications", NewHandler(pool).List)

	tests := []struct {
		name        string
		language    string
		wantTitle   string
		wantContent string
	}{
		{name: "Uzbek", language: "uz", wantTitle: "Guruhga taklif", wantContent: "Siz Oilam guruhiga qo'shilish uchun taklif qilindingiz"},
		{name: "Russian", language: "ru", wantTitle: "Приглашение в группу", wantContent: "Вас пригласили присоединиться к группе Oilam"},
		{name: "English", language: "en", wantTitle: "Group invitation", wantContent: "You have been invited to join Oilam"},
		{name: "English fallback", language: "de", wantTitle: "Group invitation", wantContent: "You have been invited to join Oilam"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(fiber.MethodGet, "/notifications", nil)
			request.Header.Set(fiber.HeaderAcceptLanguage, tt.language)
			response, err := app.Test(request, int((5 * time.Second).Milliseconds()))
			if err != nil {
				t.Fatalf("list notifications: %v", err)
			}
			defer response.Body.Close()

			var body responses.Envelope[[]Notification]
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatalf("decode notifications response: %v", err)
			}
			if len(body.Data) != 1 {
				t.Fatalf("notifications count = %d, want 1", len(body.Data))
			}
			item := body.Data[0]
			if item.EventType != "GROUP_INVITATION" || item.Title != tt.wantTitle || item.Content != tt.wantContent {
				t.Fatalf("notification = %#v, want event_type %q, title %q, content %q", item, "GROUP_INVITATION", tt.wantTitle, tt.wantContent)
			}
		})
	}
}
