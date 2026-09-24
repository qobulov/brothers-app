package telegram

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
)

type StartHandler interface {
	HandleBotStart(ctx context.Context, startToken string, chatID int64) error
}

type Worker struct {
	client *Client
	auth   StartHandler
}

func NewWorker(client *Client, auth StartHandler) *Worker {
	return &Worker{client: client, auth: auth}
}

func (w *Worker) Run(ctx context.Context) error {
	if w.client == nil {
		return nil
	}
	var offset int64
	retryDelay := time.Second
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		updates, err := w.client.GetUpdates(ctx, offset)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			slog.WarnContext(ctx, "telegram long poll failed", "error", err, "retry_in", retryDelay)
			if err := waitForRetry(ctx, retryDelay); err != nil {
				return err
			}
			retryDelay = min(retryDelay*2, 30*time.Second)
			continue
		}
		retryDelay = time.Second
		for _, update := range updates {
			if update.UpdateID < offset {
				continue
			}
			if err := HandleUpdate(ctx, w.auth, update); err != nil {
				slog.ErrorContext(ctx, "telegram update rejected", "update_id", update.UpdateID, "error", err)
			}
			// Advance after this update has been accepted or rejected. The
			// offset is process-local; hosted serverless deployments use webhook mode.
			offset = update.UpdateID + 1
		}
	}
}

func HandleUpdate(ctx context.Context, auth StartHandler, update Update) error {
	if auth == nil || update.Message == nil || update.Message.Chat.Type != "private" {
		return nil
	}
	text := strings.TrimSpace(update.Message.Text)
	if !strings.HasPrefix(text, "/start ") {
		return nil
	}
	startToken := strings.TrimSpace(strings.TrimPrefix(text, "/start "))
	if startToken == "" {
		return nil
	}
	return auth.HandleBotStart(ctx, startToken, update.Message.Chat.ID)
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
