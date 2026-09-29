package order

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPresent_StateAndVisibility(t *testing.T) {
	giverMember, receiverMember := uuid.New(), uuid.New()
	p := parties{giverMemberID: giverMember, receiverMemberID: receiverMember, giverUserID: uuid.New(), receiverUserID: uuid.New()}
	giver := viewer{memberID: giverMember, role: "employee"}
	receiver := viewer{memberID: receiverMember, role: "employee"}
	manager := viewer{memberID: uuid.New(), role: "manager"}
	investor := viewer{memberID: uuid.New(), role: "investor"}

	at := time.Date(2026, 9, 29, 10, 42, 0, 0, time.UTC)
	giverRow := confirmationRow{memberID: giverMember, amountUSD: 7000, feeUZS: 50000, confirmedAt: at}
	receiverRow := confirmationRow{memberID: receiverMember, amountUSD: 6950, feeUZS: 10000, confirmedAt: at}
	matchingReceiver := receiverRow
	matchingReceiver.amountUSD = 7000

	tests := []struct {
		name                            string
		status                          string
		rows                            []confirmationRow
		viewer                          viewer
		wantState                       string
		giverAmount, giverFee           *int64
		receiverAmount, receiverFee     *int64
		wantGiverStatus, wantRecvStatus string
	}{
		{name: "party has not confirmed", status: StatusPending, viewer: giver,
			wantState: StateWaitingForYou, wantGiverStatus: "pending", wantRecvStatus: "pending"},
		{name: "manager waits for parties", status: StatusPending, viewer: manager,
			wantState: StateWaitingForConfirmation, wantGiverStatus: "pending", wantRecvStatus: "pending"},
		{name: "unconfirmed party cannot see counterparty numbers", status: StatusPending, rows: []confirmationRow{receiverRow}, viewer: giver,
			wantState: StateWaitingForYou, wantGiverStatus: "pending", wantRecvStatus: "confirmed"},
		{name: "party sees own numbers", status: StatusPending, rows: []confirmationRow{receiverRow}, viewer: receiver,
			wantState: StateWaitingForConfirmation, receiverAmount: ptr(6950), receiverFee: ptr(10000), wantGiverStatus: "pending", wantRecvStatus: "confirmed"},
		{name: "mismatch shows counterparty amount but not fee", status: StatusPending, rows: []confirmationRow{giverRow, receiverRow}, viewer: giver,
			wantState: StateAmountMismatch, giverAmount: ptr(7000), giverFee: ptr(50000), receiverAmount: ptr(6950), wantGiverStatus: "confirmed", wantRecvStatus: "confirmed"},
		{name: "manager sees every number", status: StatusPending, rows: []confirmationRow{giverRow, receiverRow}, viewer: manager,
			wantState: StateAmountMismatch, giverAmount: ptr(7000), giverFee: ptr(50000), receiverAmount: ptr(6950), receiverFee: ptr(10000), wantGiverStatus: "confirmed", wantRecvStatus: "confirmed"},
		{name: "investor sees every number", status: StatusPending, rows: []confirmationRow{giverRow}, viewer: investor,
			wantState: StateWaitingForConfirmation, giverAmount: ptr(7000), giverFee: ptr(50000), wantGiverStatus: "confirmed", wantRecvStatus: "pending"},
		{name: "completed order reports its status", status: StatusCompleted, rows: []confirmationRow{giverRow, matchingReceiver}, viewer: receiver,
			wantState: StatusCompleted, giverAmount: ptr(7000), receiverAmount: ptr(7000), receiverFee: ptr(10000), wantGiverStatus: "confirmed", wantRecvStatus: "confirmed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, confirmations := present(tt.status, p, tt.rows, tt.viewer)
			if state != tt.wantState {
				t.Fatalf("state = %q, want %q", state, tt.wantState)
			}
			if len(confirmations) != 2 || confirmations[0].Role != RoleGiver || confirmations[1].Role != RoleReceiver {
				t.Fatalf("confirmations = %#v, want giver then receiver", confirmations)
			}
			if confirmations[0].UserID != p.giverUserID || confirmations[1].UserID != p.receiverUserID {
				t.Fatalf("confirmation users = %s/%s", confirmations[0].UserID, confirmations[1].UserID)
			}
			if confirmations[0].Status != tt.wantGiverStatus || confirmations[1].Status != tt.wantRecvStatus {
				t.Fatalf("statuses = %s/%s, want %s/%s", confirmations[0].Status, confirmations[1].Status, tt.wantGiverStatus, tt.wantRecvStatus)
			}
			assertPtr(t, "giver amount", confirmations[0].AmountUSD, tt.giverAmount)
			assertPtr(t, "giver fee", confirmations[0].FeeUZS, tt.giverFee)
			assertPtr(t, "receiver amount", confirmations[1].AmountUSD, tt.receiverAmount)
			assertPtr(t, "receiver fee", confirmations[1].FeeUZS, tt.receiverFee)
		})
	}
}

func ptr(value int64) *int64 { return &value }

func assertPtr(t *testing.T, name string, got, want *int64) {
	t.Helper()
	switch {
	case got == nil && want == nil:
	case got == nil || want == nil:
		t.Fatalf("%s = %v, want %v", name, got, want)
	case *got != *want:
		t.Fatalf("%s = %d, want %d", name, *got, *want)
	}
}
