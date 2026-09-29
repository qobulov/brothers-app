package order

import (
	"context"
	"errors"
	"testing"

	"github.com/qobulov/brothers-app/pkg/apperror"
)

func TestListEvents_HistoryWithoutConfirmationNumbers(t *testing.T) {
	f := newFixture(t)
	created := f.create(f.giver)
	f.confirm(f.receiver, created.ID, 7000, 25000)
	f.confirm(f.giver, created.ID, 6950, 50000)
	f.confirm(f.giver, created.ID, 7000, 50000)

	events, err := f.service.ListEvents(context.Background(), f.receiver, f.groupID, created.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	want := []struct{ eventType, actor string }{
		{eventCreated, "javohir"},
		{eventConfirmed, "aziz"},
		{eventConfirmed, "javohir"},
		{eventAmountMismatch, "javohir"},
		{eventConfirmationCorrected, "javohir"},
		{eventCompleted, "javohir"},
	}
	if len(events) != len(want) {
		t.Fatalf("events = %d, want %d", len(events), len(want))
	}
	for i, event := range events {
		if event.EventType != want[i].eventType || event.Actor.Name != want[i].actor {
			t.Fatalf("event %d = %s by %s, want %s by %s", i, event.EventType, event.Actor.Name, want[i].eventType, want[i].actor)
		}
		if event.EventType != eventCompleted && len(event.Payload) != 0 {
			t.Fatalf("event %s payload = %v, want empty so no confirmation numbers leak", event.EventType, event.Payload)
		}
	}
	if events[5].Payload["amount_usd"] != float64(7000) {
		t.Fatalf("completed payload = %v, want amount_usd 7000", events[5].Payload)
	}

	if _, err := f.service.ListEvents(context.Background(), f.bystander, f.groupID, created.ID); !errors.Is(err, apperror.ErrRecordNotFound) {
		t.Fatalf("bystander events error = %v, want not found", err)
	}
}
