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

func TestHandler_ListPagesNewestFirstAndSkipsMalformedPayloads(t *testing.T) {
	pool, cleanup := database.SetupTestDB(t)
	defer cleanup()

	userID := uuid.New()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO users (id, email, username, language, is_active, created_at, updated_at)
		VALUES ($1, 'paging@example.com', 'paging-user', 'en', true, now(), now())
	`, userID)
	if err != nil {
		t.Fatalf("create notification user: %v", err)
	}
	insert := func(title, payload string, age time.Duration) {
		t.Helper()
		_, err := pool.Exec(context.Background(), `
			WITH notification AS (
				INSERT INTO notifications (title_en, title_uz, title_ru, content_en, content_uz, content_ru, type, payload)
				VALUES ($1, $1, $1, '', '', '', 'TARGETED'::notification_type, $2::jsonb)
				RETURNING id
			)
			INSERT INTO notification_recipients (notification_id, user_id, created_at)
			SELECT id, $3, now() - $4::interval FROM notification
		`, title, payload, userID, age.String())
		if err != nil {
			t.Fatalf("create notification %s: %v", title, err)
		}
	}
	insert("oldest", `{"event_type":"MEMBER_JOINED"}`, 3*time.Hour)
	insert("malformed", `{"group_id":"missing event type"}`, 2*time.Hour)
	insert("newest", `{"event_type":"MEMBER_JOINED"}`, time.Hour)

	app := fiber.New()
	responses.Middleware(app, "test")
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("auth_user_id", userID)
		return c.Next()
	})
	app.Get("/notifications", NewHandler(pool).List)

	list := func(query string) (int, []Notification) {
		t.Helper()
		response, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/notifications"+query, nil), int((5 * time.Second).Milliseconds()))
		if err != nil {
			t.Fatalf("list notifications: %v", err)
		}
		defer response.Body.Close()
		if response.StatusCode != fiber.StatusOK {
			return response.StatusCode, nil
		}
		var body responses.Envelope[[]Notification]
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			t.Fatalf("decode notifications response: %v", err)
		}
		return response.StatusCode, body.Data
	}

	status, items := list("")
	if status != fiber.StatusOK || len(items) != 2 || items[0].Title != "newest" || items[1].Title != "oldest" {
		t.Fatalf("status %d, items %#v; want newest then oldest without the malformed row", status, items)
	}
	status, items = list("?limit=1&offset=2")
	if status != fiber.StatusOK || len(items) != 1 || items[0].Title != "oldest" {
		t.Fatalf("status %d, page %#v; want only oldest", status, items)
	}
	for _, query := range []string{"?limit=0", "?limit=101", "?limit=abc", "?offset=-1"} {
		if status, _ := list(query); status != fiber.StatusBadRequest {
			t.Fatalf("%s status = %d, want 400", query, status)
		}
	}
}
