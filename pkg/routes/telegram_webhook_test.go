package routes

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

type webhookStartHandler struct {
	token  string
	chatID int64
	calls  int
}

func (h *webhookStartHandler) HandleBotStart(_ context.Context, token string, chatID int64) error {
	h.token = token
	h.chatID = chatID
	h.calls++
	return nil
}

func TestTelegramWebhookHandler(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		provided   string
		body       string
		wantStatus int
		wantCalls  int
		wantToken  string
		wantChatID int64
	}{
		{
			name:       "webhook disabled without configured secret",
			body:       `{}`,
			wantStatus: fiber.StatusNotFound,
		},
		{
			name:       "rejects invalid secret",
			configured: "server-secret",
			provided:   "wrong-secret",
			body:       `{}`,
			wantStatus: fiber.StatusUnauthorized,
		},
		{
			name:       "rejects malformed update",
			configured: "server-secret",
			provided:   "server-secret",
			body:       `{`,
			wantStatus: fiber.StatusBadRequest,
		},
		{
			name:       "dispatches private start update",
			configured: "server-secret",
			provided:   "server-secret",
			body:       `{"update_id":7,"message":{"chat":{"id":42,"type":"private"},"text":"/start deep-link-token"}}`,
			wantStatus: fiber.StatusOK,
			wantCalls:  1,
			wantToken:  "deep-link-token",
			wantChatID: 42,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			auth := &webhookStartHandler{}
			app := fiber.New()
			app.Post("/webhook", telegramWebhookHandler(test.configured, auth))
			request := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString(test.body))
			request.Header.Set("Content-Type", "application/json")
			if test.provided != "" {
				request.Header.Set(telegramWebhookSecretHeader, test.provided)
			}

			response, err := app.Test(request)
			if err != nil {
				t.Fatalf("webhook request: %v", err)
			}
			defer response.Body.Close()
			if response.StatusCode != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.StatusCode, test.wantStatus)
			}
			if auth.calls != test.wantCalls || auth.token != test.wantToken || auth.chatID != test.wantChatID {
				t.Fatalf("handler state = calls:%d token:%q chat:%d", auth.calls, auth.token, auth.chatID)
			}
		})
	}
}
