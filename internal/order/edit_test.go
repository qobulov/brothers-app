package order

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/apperror"
)

func TestEdit_ClearsConfirmationsAndRecordsDiff(t *testing.T) {
	f := newFixture(t)
	created := f.create(f.giver)
	f.confirm(f.receiver, created.ID, 7000, 0)

	amount, fee := int64(6800), int64(40000)
	edited, err := f.service.Edit(context.Background(), f.giver, f.groupID, created.ID, EditInput{AmountUSD: &amount, FeeUZS: &fee})
	if err != nil {
		t.Fatalf("edit order: %v", err)
	}
	if edited.AmountUSD != 6800 || edited.FeeUZS != 40000 || edited.State != StateWaitingForYou {
		t.Fatalf("edited order = %d/%d/%s", edited.AmountUSD, edited.FeeUZS, edited.State)
	}
	for _, confirmation := range edited.Confirmations {
		if confirmation.Status != "pending" {
			t.Fatalf("confirmation after edit = %#v, want pending", confirmation)
		}
	}

	var payload []byte
	err = f.pool.QueryRow(context.Background(), `
		SELECT payload FROM order_events WHERE order_id = $1 AND event_type = 'updated'
	`, created.ID).Scan(&payload)
	if err != nil {
		t.Fatalf("read updated event: %v", err)
	}
	var decoded struct {
		Changes map[string]struct {
			Old any `json:"old"`
			New any `json:"new"`
		} `json:"changes"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode updated event: %v", err)
	}
	if len(decoded.Changes) != 2 || decoded.Changes["amount_usd"].Old != float64(7000) || decoded.Changes["amount_usd"].New != float64(6800) {
		t.Fatalf("changes = %#v, want amount_usd 7000 -> 6800 and fee_uzs", decoded.Changes)
	}
	if f.notificationCount(f.receiver, notifyOrderUpdated) != 1 || f.notificationCount(f.giver, notifyOrderUpdated) != 0 {
		t.Fatal("an edit must notify only the other party")
	}
}

func TestEdit_NoOpWritesNothing(t *testing.T) {
	f := newFixture(t)
	created := f.create(f.giver)
	f.confirm(f.receiver, created.ID, 7000, 0)
	same, phone := int64(7000), "998901111111"

	edited, err := f.service.Edit(context.Background(), f.giver, f.groupID, created.ID, EditInput{AmountUSD: &same, GiverCustomerPhone: &phone})
	if err != nil {
		t.Fatalf("no-op edit: %v", err)
	}
	if edited.Confirmations[1].Status != "confirmed" {
		t.Fatal("a no-op edit must keep confirmations")
	}
	if f.eventCount(created.ID, eventUpdated) != 0 || f.notificationCount(f.receiver, notifyOrderUpdated) != 0 {
		t.Fatal("a no-op edit must not write events or notifications")
	}
}

func TestEdit_Rules(t *testing.T) {
	f := newFixture(t)
	created := f.create(f.giver)
	ctx := context.Background()
	zero, amount := int64(0), int64(6000)
	tests := []struct {
		name    string
		actor   uuid.UUID
		input   EditInput
		wantErr error
	}{
		{name: "empty edit", actor: f.giver, input: EditInput{}, wantErr: apperror.ErrInvalidData},
		{name: "invalid amount", actor: f.giver, input: EditInput{AmountUSD: &zero}, wantErr: apperror.ErrInvalidData},
		{name: "investor", actor: f.investor, input: EditInput{AmountUSD: &amount}, wantErr: apperror.ErrForbidden},
		{name: "employee not on the order", actor: f.bystander, input: EditInput{AmountUSD: &amount}, wantErr: apperror.ErrRecordNotFound},
		{name: "employee editing themselves out", actor: f.giver, input: EditInput{GiverUserID: &f.bystander}, wantErr: apperror.ErrForbidden},
		{name: "party that is not an employee", actor: f.manager, input: EditInput{GiverUserID: &f.investor}, wantErr: apperror.ErrRecordNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := f.service.Edit(ctx, tt.actor, f.groupID, created.ID, tt.input); !errors.Is(err, tt.wantErr) {
				t.Fatalf("edit error = %v, want %v", err, tt.wantErr)
			}
		})
	}

	replaced, err := f.service.Edit(ctx, f.manager, f.groupID, created.ID, EditInput{GiverUserID: &f.bystander})
	if err != nil {
		t.Fatalf("manager replaces giver: %v", err)
	}
	if replaced.Giver.UserID != f.bystander || replaced.Giver.Location != nil {
		t.Fatalf("new giver = %#v, want bystander without a location", replaced.Giver)
	}
}

func TestEdit_SwapPartiesResnapshotsLocations(t *testing.T) {
	f := newFixture(t)
	created := f.create(f.manager)
	f.confirm(f.giver, created.ID, 7000, 0)

	swapped, err := f.service.Edit(context.Background(), f.manager, f.groupID, created.ID, EditInput{GiverUserID: &f.receiver, ReceiverUserID: &f.giver})
	if err != nil {
		t.Fatalf("swap parties: %v", err)
	}
	if swapped.Giver.UserID != f.receiver || swapped.Giver.Location == nil || swapped.Giver.Location.Name != "Kokand" {
		t.Fatalf("giver after swap = %#v, want Aziz in Kokand", swapped.Giver)
	}
	if swapped.Receiver.UserID != f.giver || swapped.Receiver.Location == nil || swapped.Receiver.Location.Name != "Tashkent" {
		t.Fatalf("receiver after swap = %#v, want Javohir in Tashkent", swapped.Receiver)
	}
	if swapped.Confirmations[0].Status != "pending" || swapped.Confirmations[1].Status != "pending" {
		t.Fatal("swapping parties must clear confirmations")
	}
}

func TestEdit_CompletedOrderIsLocked(t *testing.T) {
	f := newFixture(t)
	created := f.create(f.manager)
	f.confirm(f.giver, created.ID, 7000, 0)
	f.confirm(f.receiver, created.ID, 7000, 0)
	amount := int64(1)

	_, err := f.service.Edit(context.Background(), f.manager, f.groupID, created.ID, EditInput{AmountUSD: &amount})
	if !errors.Is(err, apperror.ErrConflict) {
		t.Fatalf("edit completed order error = %v, want conflict", err)
	}
}
