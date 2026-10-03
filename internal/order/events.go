package order

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/database"
)

const (
	eventCreated               = "created"
	eventUpdated               = "updated"
	eventConfirmed             = "confirmed"
	eventConfirmationCorrected = "confirmation_corrected"
	eventAmountMismatch        = "amount_mismatch"
	eventCompleted             = "completed"
	eventCancellationRequested = "cancellation_requested"
	eventCancellationRejected  = "cancellation_rejected"
	eventCancelled             = "cancelled"
)

// orderEvent payloads must never carry confirmation amounts or fees: both
// parties read the history, and it would leak the counterparty's numbers.
type orderEvent struct {
	orderID   uuid.UUID
	actorID   uuid.UUID
	eventType string
	payload   any
}

func writeEvent(ctx context.Context, q database.Querier, event orderEvent) error {
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

// ListEvents returns an order's history, oldest first.
func (s *Service) ListEvents(ctx context.Context, actorID, groupID, orderID uuid.UUID) ([]Event, error) {
	v, err := loadViewer(ctx, s.pool, groupID, actorID)
	if err != nil {
		return nil, err
	}
	_, p, err := loadOrder(ctx, s.pool, groupID, orderID)
	if err != nil {
		return nil, err
	}
	if !v.canSee(p) {
		return nil, apperror.ErrRecordNotFound
	}
	rows, err := s.pool.Query(ctx, orderEventsQuery, orderID)
	if err != nil {
		return nil, fmt.Errorf("listing order events: %w", err)
	}
	defer rows.Close()
	events := make([]Event, 0)
	for rows.Next() {
		var event Event
		var payload []byte
		if err := rows.Scan(&event.ID, &event.EventType, &event.Actor.UserID, &event.Actor.Name, &payload, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning order event: %w", err)
		}
		if err := json.Unmarshal(payload, &event.Payload); err != nil {
			return nil, fmt.Errorf("decoding order event payload: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating order events: %w", err)
	}
	return events, nil
}

var orderEventsQuery = fmt.Sprintf(`
	SELECT events.id, events.event_type, events.actor_user_id, %s, events.payload, events.created_at
	FROM order_events events
	JOIN users ON users.id = events.actor_user_id
	WHERE events.order_id = $1 AND events.deleted_at IS NULL
	ORDER BY events.created_at, events.id
`, displayName("users"))
