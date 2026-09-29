package order

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/apperror"
)

func TestCreate_PermissionMatrix(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name    string
		actor   uuid.UUID
		wantErr error
	}{
		{name: "owner", actor: f.owner},
		{name: "manager", actor: f.manager},
		{name: "employee who is a party", actor: f.giver},
		{name: "employee who is not a party", actor: f.bystander, wantErr: apperror.ErrForbidden},
		{name: "investor", actor: f.investor, wantErr: apperror.ErrForbidden},
		{name: "not a member", actor: f.stranger, wantErr: apperror.ErrRecordNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.service.Create(context.Background(), tt.actor, f.groupID, f.createInput())
			if tt.wantErr == nil && err != nil {
				t.Fatalf("create: %v", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("create error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestCreate_RejectsInvalidInput(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name    string
		change  func(*CreateInput)
		wantErr error
	}{
		{name: "zero amount", change: func(in *CreateInput) { in.AmountUSD = 0 }, wantErr: apperror.ErrInvalidData},
		{name: "amount above limit", change: func(in *CreateInput) { in.AmountUSD = maxAmountUSD + 1 }, wantErr: apperror.ErrInvalidData},
		{name: "negative fee", change: func(in *CreateInput) { in.FeeUZS = -1 }, wantErr: apperror.ErrInvalidData},
		{name: "fee above limit", change: func(in *CreateInput) { in.FeeUZS = maxFeeUZS + 1 }, wantErr: apperror.ErrInvalidData},
		{name: "same giver and receiver", change: func(in *CreateInput) { in.ReceiverUserID = in.GiverUserID }, wantErr: apperror.ErrInvalidData},
		{name: "invalid giver phone", change: func(in *CreateInput) { in.GiverCustomerPhone = "abc" }, wantErr: apperror.ErrInvalidData},
		{name: "missing receiver phone", change: func(in *CreateInput) { in.ReceiverCustomerPhone = "" }, wantErr: apperror.ErrInvalidData},
		{name: "manager as a party", change: func(in *CreateInput) { in.GiverUserID = f.manager }, wantErr: apperror.ErrRecordNotFound},
		{name: "unknown user as a party", change: func(in *CreateInput) { in.ReceiverUserID = uuid.New() }, wantErr: apperror.ErrRecordNotFound},
		{name: "outsider as a party", change: func(in *CreateInput) { in.ReceiverUserID = f.stranger }, wantErr: apperror.ErrRecordNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := f.createInput()
			tt.change(&input)
			if _, err := f.service.Create(context.Background(), f.manager, f.groupID, input); !errors.Is(err, tt.wantErr) {
				t.Fatalf("create error = %v, want %v", err, tt.wantErr)
			}
		})
	}
	if n := f.count(`SELECT COUNT(*) FROM orders`); n != 0 {
		t.Fatalf("orders = %d, want none after rejected input", n)
	}
}

func TestCreate_ReturnsDetailWithCustomersAndLocationSnapshots(t *testing.T) {
	f := newFixture(t)
	created := f.create(f.giver)

	if created.Status != StatusPending || created.State != StateWaitingForYou {
		t.Fatalf("status/state = %s/%s, want pending/waiting_for_you", created.Status, created.State)
	}
	if created.AmountUSD != 7000 || created.FeeUZS != 50000 || created.CreatedBy.UserID != f.giver {
		t.Fatalf("order = %#v", created)
	}
	if created.Giver.UserID != f.giver || created.Giver.Name != "javohir" || created.Giver.CustomerPhone != "+998901111111" {
		t.Fatalf("giver = %#v", created.Giver)
	}
	if created.Giver.Location == nil || created.Giver.Location.Name != "Tashkent" {
		t.Fatalf("giver location = %#v, want Tashkent", created.Giver.Location)
	}
	if created.Receiver.Location == nil || created.Receiver.Location.Name != "Kokand" || created.Receiver.CustomerPhone != "+998902222222" {
		t.Fatalf("receiver = %#v", created.Receiver)
	}
	if len(created.Confirmations) != 2 || created.Confirmations[0].Status != "pending" || created.Confirmations[1].Status != "pending" {
		t.Fatalf("confirmations = %#v, want two pending", created.Confirmations)
	}

	// The same phones typed differently must reuse the same customers.
	second := f.createInput()
	second.GiverCustomerPhone = "998901111111"
	second.ReceiverCustomerPhone = "+998 (90) 222-22-22"
	if _, err := f.service.Create(context.Background(), f.giver, f.groupID, second); err != nil {
		t.Fatalf("create second order: %v", err)
	}
	if n := f.count(`SELECT COUNT(*) FROM customers WHERE group_id = $1`, f.groupID); n != 2 {
		t.Fatalf("customers = %d, want 2", n)
	}

	// Removing a location later must not rewrite the order's history.
	f.exec(`UPDATE locations SET employee_id = NULL, deleted_at = now() WHERE id = $1`, f.tashkentID)
	reloaded, err := f.service.Get(context.Background(), f.manager, f.groupID, created.ID)
	if err != nil {
		t.Fatalf("get order: %v", err)
	}
	if reloaded.Giver.Location == nil || reloaded.Giver.Location.Name != "Tashkent" {
		t.Fatalf("giver location after deletion = %#v, want Tashkent", reloaded.Giver.Location)
	}
}

func TestCreate_WritesEventAndNotifiesOtherParties(t *testing.T) {
	f := newFixture(t)
	byGiver := f.create(f.giver)
	if n := f.eventCount(byGiver.ID, eventCreated); n != 1 {
		t.Fatalf("created events = %d, want 1", n)
	}
	if f.notificationCount(f.receiver, notifyOrderCreated) != 1 || f.notificationCount(f.giver, notifyOrderCreated) != 0 {
		t.Fatal("giver-created order must notify only the receiver")
	}

	f.create(f.manager)
	if f.notificationCount(f.receiver, notifyOrderCreated) != 2 || f.notificationCount(f.giver, notifyOrderCreated) != 1 {
		t.Fatal("manager-created order must notify both parties")
	}
	if f.notificationCount(f.manager, notifyOrderCreated) != 0 {
		t.Fatal("the creating manager must not be notified")
	}
}

func TestGet_Visibility(t *testing.T) {
	f := newFixture(t)
	created := f.create(f.manager)
	for _, actor := range []uuid.UUID{f.owner, f.manager, f.investor, f.giver, f.receiver} {
		if _, err := f.service.Get(context.Background(), actor, f.groupID, created.ID); err != nil {
			t.Fatalf("get as %s: %v", actor, err)
		}
	}
	for _, actor := range []uuid.UUID{f.bystander, f.stranger} {
		if _, err := f.service.Get(context.Background(), actor, f.groupID, created.ID); !errors.Is(err, apperror.ErrRecordNotFound) {
			t.Fatalf("get as outsider error = %v, want not found", err)
		}
	}
	if _, err := f.service.Get(context.Background(), f.manager, f.groupID, uuid.New()); !errors.Is(err, apperror.ErrRecordNotFound) {
		t.Fatalf("unknown order error = %v, want not found", err)
	}
}
