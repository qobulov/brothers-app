package telegram

import (
	"context"
	"errors"
	"testing"
	"time"
)

type startHandlerStub struct {
	token  string
	chatID int64
	calls  int
}

func (h *startHandlerStub) HandleBotStart(_ context.Context, token string, chatID int64) error {
	h.token = token
	h.chatID = chatID
	h.calls++
	return nil
}

func TestHandleUpdate(t *testing.T) {
	auth := &startHandlerStub{}
	update := Update{Message: &Message{Chat: Chat{ID: 91, Type: "private"}, Text: "/start token-123"}}

	if err := HandleUpdate(t.Context(), auth, update); err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}
	if auth.calls != 1 || auth.token != "token-123" || auth.chatID != 91 {
		t.Fatalf("handler state = calls:%d token:%q chat:%d", auth.calls, auth.token, auth.chatID)
	}
}

func TestWaitForRetryStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := waitForRetry(ctx, time.Hour)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForRetry() error = %v, want context.Canceled", err)
	}
}
