package order

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

const (
	eventCreated               = "created"
	eventUpdated               = "updated"
	eventConfirmed             = "confirmed"
	eventConfirmationCorrected = "confirmation_corrected"
	eventAmountMismatch        = "amount_mismatch"
	eventCompleted             = "completed"
)

// orderEvent payloads must never carry confirmation amounts or fees: both
// parties read the history, and it would leak the counterparty's numbers.
type orderEvent struct {
	orderID   uuid.UUID
	actorID   uuid.UUID
	eventType string
	payload   any
}

func writeEvent(ctx context.Context, q querier, event orderEvent) error {
	payload := event.payload
	if payload == nil {
		payload = map[string]any{}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encoding order event: %w", err)
	}
	// clock_timestamp keeps several events from one transaction in order.
	_, err = q.Exec(ctx, `
		INSERT INTO order_events (order_id, actor_user_id, event_type, payload, created_at, updated_at)
		VALUES ($1, $2, $3, $4, clock_timestamp(), clock_timestamp())
	`, event.orderID, event.actorID, event.eventType, encoded)
	if err != nil {
		return fmt.Errorf("writing order %s event: %w", event.eventType, err)
	}
	return nil
}
