package order

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/apperror"
)

func (f *fixture) confirm(actorID, orderID uuid.UUID, amountUSD, feeUZS int64) Order {
	f.t.Helper()
	confirmed, err := f.service.Confirm(context.Background(), actorID, f.groupID, orderID, ConfirmInput{AmountUSD: amountUSD, FeeUZS: feeUZS})
	if err != nil {
		f.t.Fatalf("confirm order: %v", err)
	}
	return confirmed
}

func (f *fixture) balance(memberID uuid.UUID) int64 {
	f.t.Helper()
	var balance int64
	err := f.pool.QueryRow(context.Background(), `
		SELECT COALESCE(SUM(balance_usd), 0) FROM employee_balances WHERE member_id = $1 AND deleted_at IS NULL
	`, memberID).Scan(&balance)
	if err != nil {
		f.t.Fatalf("read balance: %v", err)
	}
	return balance
}

func (f *fixture) profit(memberID uuid.UUID, year, month int) int64 {
	f.t.Helper()
	var profit int64
	err := f.pool.QueryRow(context.Background(), `
		SELECT COALESCE(SUM(profit_uzs), 0) FROM member_profit_periods
		WHERE member_id = $1 AND year = $2 AND month = $3 AND deleted_at IS NULL
	`, memberID, year, month).Scan(&profit)
	if err != nil {
		f.t.Fatalf("read profit: %v", err)
	}
	return profit
}

func TestConfirm_MatchingAmountsCompleteAndApplyEffects(t *testing.T) {
	f := newFixture(t)
	f.exec(`INSERT INTO employee_balances (group_id, member_id, balance_usd) VALUES ($1, $2, 100)`, f.groupID, f.giverMember)
	created := f.create(f.manager)
	// 20:00 UTC on 30 September is 01:00 on 1 October in Tashkent.
	f.service.now = func() time.Time { return time.Date(2026, 9, 30, 20, 0, 0, 0, time.UTC) }

	afterGiver := f.confirm(f.giver, created.ID, 7000, 50000)
	if afterGiver.Status != StatusPending || afterGiver.State != StateWaitingForConfirmation {
		t.Fatalf("after first confirmation = %s/%s, want pending/waiting_for_confirmation", afterGiver.Status, afterGiver.State)
	}
	completed := f.confirm(f.receiver, created.ID, 7000, 10000)
	if completed.Status != StatusCompleted || completed.State != StatusCompleted || completed.CompletedAt == nil {
		t.Fatalf("after both confirmations = %s/%s completed_at=%v", completed.Status, completed.State, completed.CompletedAt)
	}

	if got := f.balance(f.giverMember); got != 7100 {
		t.Fatalf("giver balance = %d, want 7100", got)
	}
	if got := f.balance(f.receiverMember); got != -7000 {
		t.Fatalf("receiver balance = %d, want -7000", got)
	}
	if f.profit(f.giverMember, 2026, 10) != 50000 || f.profit(f.receiverMember, 2026, 10) != 10000 {
		t.Fatal("each party's fee must go to their own October profit")
	}
	if f.profit(f.giverMember, 2026, 9) != 0 {
		t.Fatal("profit must use the Tashkent month, not the UTC month")
	}
	if f.eventCount(created.ID, eventConfirmed) != 2 || f.eventCount(created.ID, eventCompleted) != 1 {
		t.Fatal("want two confirmed events and one completed event")
	}
	if f.notificationCount(f.giver, notifyOrderCompleted) != 1 || f.notificationCount(f.receiver, notifyOrderCompleted) != 1 {
		t.Fatal("both parties must be notified of completion")
	}

	_, err := f.service.Confirm(context.Background(), f.giver, f.groupID, created.ID, ConfirmInput{AmountUSD: 7000})
	if !errors.Is(err, apperror.ErrConflict) {
		t.Fatalf("confirm completed order error = %v, want conflict", err)
	}
}

func TestConfirm_MismatchThenCorrection(t *testing.T) {
	f := newFixture(t)
	created := f.create(f.manager)
	f.confirm(f.giver, created.ID, 7000, 0)
	mismatch := f.confirm(f.receiver, created.ID, 6950, 0)

	if mismatch.Status != StatusPending || mismatch.State != StateAmountMismatch {
		t.Fatalf("after mismatch = %s/%s, want pending/amount_mismatch", mismatch.Status, mismatch.State)
	}
	if mismatch.Confirmations[0].AmountUSD == nil || *mismatch.Confirmations[0].AmountUSD != 7000 {
		t.Fatal("after confirming, the receiver must see the giver's amount")
	}
	if f.balance(f.giverMember) != 0 || f.balance(f.receiverMember) != 0 {
		t.Fatal("a mismatch must not change balances")
	}
	if f.notificationCount(f.giver, notifyOrderAmountMismatch) != 1 || f.notificationCount(f.receiver, notifyOrderAmountMismatch) != 1 {
		t.Fatal("both parties must be notified of the mismatch")
	}

	corrected := f.confirm(f.receiver, created.ID, 7000, 0)
	if corrected.Status != StatusCompleted {
		t.Fatalf("after correction status = %s, want completed", corrected.Status)
	}
	if f.eventCount(created.ID, eventConfirmationCorrected) != 1 || f.eventCount(created.ID, eventAmountMismatch) != 1 {
		t.Fatal("want one correction and one mismatch event")
	}
	if f.balance(f.giverMember) != 7000 || f.balance(f.receiverMember) != -7000 {
		t.Fatal("completion after correction must apply the corrected amount")
	}
}

func TestConfirm_Rules(t *testing.T) {
	f := newFixture(t)
	created := f.create(f.manager)
	ctx := context.Background()
	tests := []struct {
		name    string
		actor   uuid.UUID
		input   ConfirmInput
		wantErr error
	}{
		{name: "manager is not a party", actor: f.manager, input: ConfirmInput{AmountUSD: 7000}, wantErr: apperror.ErrForbidden},
		{name: "investor is not a party", actor: f.investor, input: ConfirmInput{AmountUSD: 7000}, wantErr: apperror.ErrForbidden},
		{name: "other employee cannot see the order", actor: f.bystander, input: ConfirmInput{AmountUSD: 7000}, wantErr: apperror.ErrRecordNotFound},
		{name: "not a member", actor: f.stranger, input: ConfirmInput{AmountUSD: 7000}, wantErr: apperror.ErrRecordNotFound},
		{name: "zero amount", actor: f.giver, input: ConfirmInput{AmountUSD: 0}, wantErr: apperror.ErrInvalidData},
		{name: "negative fee", actor: f.giver, input: ConfirmInput{AmountUSD: 7000, FeeUZS: -1}, wantErr: apperror.ErrInvalidData},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := f.service.Confirm(ctx, tt.actor, f.groupID, created.ID, tt.input); !errors.Is(err, tt.wantErr) {
				t.Fatalf("confirm error = %v, want %v", err, tt.wantErr)
			}
		})
	}
	if _, err := f.service.Confirm(ctx, f.giver, f.groupID, uuid.New(), ConfirmInput{AmountUSD: 7000}); !errors.Is(err, apperror.ErrRecordNotFound) {
		t.Fatalf("unknown order error = %v, want not found", err)
	}
}

func TestConfirm_PartyRemovedFromGroup(t *testing.T) {
	f := newFixture(t)
	created := f.create(f.manager)
	f.exec(`UPDATE group_members SET deleted_at = now() WHERE id = $1`, f.receiverMember)

	_, err := f.service.Confirm(context.Background(), f.receiver, f.groupID, created.ID, ConfirmInput{AmountUSD: 7000})
	if !errors.Is(err, apperror.ErrRecordNotFound) {
		t.Fatalf("removed party confirm error = %v, want not found", err)
	}
	reloaded, err := f.service.Get(context.Background(), f.manager, f.groupID, created.ID)
	if err != nil || reloaded.Status != StatusPending {
		t.Fatalf("order after removal = %#v, %v; want still pending", reloaded.Status, err)
	}
}

func TestConfirm_ConcurrentConfirmationsApplyOnce(t *testing.T) {
	f := newFixture(t)
	created := f.create(f.manager)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, actor := range []uuid.UUID{f.giver, f.receiver} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = f.service.Confirm(context.Background(), actor, f.groupID, created.ID, ConfirmInput{AmountUSD: 7000})
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("concurrent confirm: %v", err)
		}
	}
	if f.balance(f.giverMember) != 7000 || f.balance(f.receiverMember) != -7000 {
		t.Fatalf("balances = %d/%d, want 7000/-7000", f.balance(f.giverMember), f.balance(f.receiverMember))
	}
	if n := f.eventCount(created.ID, eventCompleted); n != 1 {
		t.Fatalf("completed events = %d, want 1", n)
	}
}
