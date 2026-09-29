package order

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/apperror"
)

func (f *fixture) requestCancellation(actorID, orderID uuid.UUID) Order {
	f.t.Helper()
	requested, err := f.service.RequestCancellation(context.Background(), actorID, f.groupID, orderID, "customer changed their mind")
	if err != nil {
		f.t.Fatalf("request cancellation: %v", err)
	}
	return requested
}

func (f *fixture) respondCancellation(actorID, orderID uuid.UUID, action string) Order {
	f.t.Helper()
	responded, err := f.service.RespondCancellation(context.Background(), actorID, f.groupID, orderID, action)
	if err != nil {
		f.t.Fatalf("%s cancellation: %v", action, err)
	}
	return responded
}

func TestCancellation_ReceiverCancelsCompletedOrderAndReversesEffects(t *testing.T) {
	f := newFixture(t)
	created := f.create(f.manager)
	f.service.now = func() time.Time { return time.Date(2026, 9, 30, 20, 0, 0, 0, time.UTC) } // October in Tashkent
	f.confirm(f.giver, created.ID, 7000, 50000)
	f.confirm(f.receiver, created.ID, 7000, 10000)

	f.service.now = func() time.Time { return time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC) }
	requested := f.requestCancellation(f.receiver, created.ID)
	if requested.Status != StatusCompleted || requested.State != StateCancellationRequested {
		t.Fatalf("after request = %s/%s, want completed/cancellation_requested", requested.Status, requested.State)
	}
	c := requested.Cancellation
	if c == nil || c.Status != cancellationPending || c.RequestedBy.UserID != f.receiver || c.Reason != "customer changed their mind" {
		t.Fatalf("cancellation = %#v", c)
	}
	if c.Approvals[0].Status != "waiting" || c.Approvals[1].Status != "approved" {
		t.Fatalf("approvals = %#v, want giver waiting and receiver approved", c.Approvals)
	}
	if f.notificationCount(f.giver, notifyOrderCancellationRequested) != 1 || f.notificationCount(f.receiver, notifyOrderCancellationRequested) != 0 {
		t.Fatal("only the other party must be asked to approve")
	}
	items, _ := f.service.List(context.Background(), f.giver, f.groupID, ListInput{})
	if items[0].State != StateCancellationRequested {
		t.Fatalf("list state = %s, want cancellation_requested", items[0].State)
	}

	cancelled := f.respondCancellation(f.giver, created.ID, CancellationApprove)
	if cancelled.Status != StatusCancelled || cancelled.State != StatusCancelled {
		t.Fatalf("after approval = %s/%s, want cancelled", cancelled.Status, cancelled.State)
	}
	if cancelled.Cancellation == nil || cancelled.Cancellation.Status != cancellationApproved || cancelled.Cancellation.Approvals[0].Status != "approved" {
		t.Fatalf("approved cancellation = %#v", cancelled.Cancellation)
	}
	if f.balance(f.giverMember) != 0 || f.balance(f.receiverMember) != 0 {
		t.Fatalf("balances after reversal = %d/%d, want 0/0", f.balance(f.giverMember), f.balance(f.receiverMember))
	}
	if f.profit(f.giverMember, 2026, 10) != 50000 || f.profit(f.giverMember, 2026, 11) != -50000 || f.profit(f.receiverMember, 2026, 11) != -10000 {
		t.Fatal("profit must be reversed in the cancellation month and kept in the month it was earned")
	}
	if f.notificationCount(f.giver, notifyOrderCancelled) != 1 || f.notificationCount(f.receiver, notifyOrderCancelled) != 1 {
		t.Fatal("both parties must be notified of the cancellation")
	}
	var reversed bool
	if err := f.pool.QueryRow(context.Background(), `SELECT (payload->>'reversed')::boolean FROM order_events WHERE order_id = $1 AND event_type = 'cancelled'`, created.ID).Scan(&reversed); err != nil || !reversed {
		t.Fatalf("cancelled event reversed = %v, %v; want true", reversed, err)
	}
}

func TestCancellation_RejectKeepsOrderThenApprovalCancelsPendingOrder(t *testing.T) {
	f := newFixture(t)
	created := f.create(f.giver)
	f.requestCancellation(f.giver, created.ID)

	kept := f.respondCancellation(f.receiver, created.ID, CancellationReject)
	if kept.Status != StatusPending || kept.Cancellation != nil || kept.State != StateWaitingForYou {
		t.Fatalf("after rejection = %s/%s cancellation=%#v", kept.Status, kept.State, kept.Cancellation)
	}
	if f.notificationCount(f.giver, notifyOrderCancellationRejected) != 1 {
		t.Fatal("the requester must be told the order was kept")
	}

	f.requestCancellation(f.giver, created.ID)
	cancelled := f.respondCancellation(f.receiver, created.ID, CancellationApprove)
	if cancelled.Status != StatusCancelled {
		t.Fatalf("status = %s, want cancelled", cancelled.Status)
	}
	if f.balance(f.giverMember) != 0 || f.balance(f.receiverMember) != 0 {
		t.Fatal("cancelling a pending order must not touch balances")
	}
	if n := f.count(`SELECT COUNT(*) FROM employee_balances WHERE group_id = $1`, f.groupID); n != 0 {
		t.Fatalf("balance rows = %d, want none", n)
	}
}

func TestCancellation_RequesterCanWithdraw(t *testing.T) {
	f := newFixture(t)
	created := f.create(f.giver)
	f.requestCancellation(f.receiver, created.ID)
	withdrawn := f.respondCancellation(f.receiver, created.ID, CancellationReject)
	if withdrawn.Status != StatusPending || withdrawn.Cancellation != nil {
		t.Fatalf("after withdrawal = %s cancellation=%#v", withdrawn.Status, withdrawn.Cancellation)
	}
}

func TestCancellation_Rules(t *testing.T) {
	f := newFixture(t)
	created := f.create(f.giver)
	ctx := context.Background()
	amount := int64(6000)

	if _, err := f.service.RequestCancellation(ctx, f.manager, f.groupID, created.ID, ""); !errors.Is(err, apperror.ErrForbidden) {
		t.Fatalf("manager request error = %v, want forbidden", err)
	}
	if _, err := f.service.RequestCancellation(ctx, f.bystander, f.groupID, created.ID, ""); !errors.Is(err, apperror.ErrRecordNotFound) {
		t.Fatalf("bystander request error = %v, want not found", err)
	}
	if _, err := f.service.RequestCancellation(ctx, f.giver, f.groupID, created.ID, strings.Repeat("x", maxCancellationReason+1)); !errors.Is(err, apperror.ErrInvalidData) {
		t.Fatalf("long reason error = %v, want invalid data", err)
	}
	if _, err := f.service.RespondCancellation(ctx, f.receiver, f.groupID, created.ID, CancellationApprove); !errors.Is(err, apperror.ErrConflict) {
		t.Fatalf("respond without request error = %v, want conflict", err)
	}

	f.requestCancellation(f.giver, created.ID)
	tests := []struct {
		name    string
		call    func() error
		wantErr error
	}{
		{name: "second request", wantErr: apperror.ErrConflict, call: func() error {
			_, err := f.service.RequestCancellation(ctx, f.receiver, f.groupID, created.ID, "")
			return err
		}},
		{name: "requester approves own request", wantErr: apperror.ErrForbidden, call: func() error {
			_, err := f.service.RespondCancellation(ctx, f.giver, f.groupID, created.ID, CancellationApprove)
			return err
		}},
		{name: "manager approves", wantErr: apperror.ErrForbidden, call: func() error {
			_, err := f.service.RespondCancellation(ctx, f.manager, f.groupID, created.ID, CancellationApprove)
			return err
		}},
		{name: "invalid action", wantErr: apperror.ErrInvalidData, call: func() error {
			_, err := f.service.RespondCancellation(ctx, f.receiver, f.groupID, created.ID, "maybe")
			return err
		}},
		{name: "confirm while request is open", wantErr: apperror.ErrConflict, call: func() error {
			_, err := f.service.Confirm(ctx, f.receiver, f.groupID, created.ID, ConfirmInput{AmountUSD: 7000})
			return err
		}},
		{name: "edit while request is open", wantErr: apperror.ErrConflict, call: func() error {
			_, err := f.service.Edit(ctx, f.giver, f.groupID, created.ID, EditInput{AmountUSD: &amount})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}

	f.respondCancellation(f.receiver, created.ID, CancellationApprove)
	if _, err := f.service.RequestCancellation(ctx, f.giver, f.groupID, created.ID, ""); !errors.Is(err, apperror.ErrConflict) {
		t.Fatalf("request on cancelled order error = %v, want conflict", err)
	}
}
