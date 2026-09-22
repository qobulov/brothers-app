package telegram

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/qobulov/brothers-app/internal/auth/service"
)

type Worker struct {
	client *Client
	auth   *service.Service
}

func NewWorker(client *Client, auth *service.Service) *Worker {
	return &Worker{client: client, auth: auth}
}

func (w *Worker) Run(ctx context.Context) error {
	if w.client == nil {
		return nil
	}
	var offset int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		updates, err := w.client.GetUpdates(ctx, offset)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			log.Printf("telegram long poll failed: %v", err)
			continue
		}
		for _, update := range updates {
			if update.UpdateID < offset {
				continue
			}
			if update.Message == nil || update.Message.Chat.Type != "private" {
				offset = update.UpdateID + 1
				continue
			}
			text := strings.TrimSpace(update.Message.Text)
			if !strings.HasPrefix(text, "/start ") {
				offset = update.UpdateID + 1
				continue
			}
			startToken := strings.TrimSpace(strings.TrimPrefix(text, "/start "))
			if startToken == "" {
				offset = update.UpdateID + 1
				continue
			}
			if err := w.auth.HandleBotStart(ctx, startToken, update.Message.Chat.ID); err != nil {
				log.Printf("telegram update %d rejected: %v", update.UpdateID, err)
			}
			// Advance only after this update has been durably accepted or rejected.
			// The offset is process-local until the persistent update-offset store is added.
			offset = update.UpdateID + 1
		}
	}
}
