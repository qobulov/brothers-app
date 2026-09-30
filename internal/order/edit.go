package order

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/apperror"
)

// Edit changes a pending order. Any real change clears both confirmations, so
// the parties confirm the new terms again.
func (s *Service) Edit(ctx context.Context, actorID, groupID, orderID uuid.UUID, input EditInput) (Order, error) {
	if input.empty() {
		return Order{}, apperror.NothingToUpdate()
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("beginning order edit: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	v, err := loadViewer(ctx, tx, groupID, actorID)
	if err != nil {
		return Order{}, err
	}
	current, err := lockOrder(ctx, tx, groupID, orderID)
	if err != nil {
		return Order{}, err
	}
	if !v.canSee(current.parties) {
		return Order{}, apperror.ErrRecordNotFound
	}
	if current.status != StatusPending {
		return Order{}, notPending(current.status)
	}
	if err := requireNoOpenCancellation(ctx, tx, orderID); err != nil {
		return Order{}, err
	}
	next, err := current.apply(input)
	if err != nil {
		return Order{}, err
	}
	// Checked against the result so an employee cannot edit themselves out.
	if err := v.canWrite(next.GiverUserID, next.ReceiverUserID); err != nil {
		return Order{}, err
	}
	changes := current.changes(next)
	if len(changes) == 0 {
		return s.Get(ctx, actorID, groupID, orderID)
	}

	edit := orderEdit{groupID: groupID, orderID: orderID, current: current, next: next, at: s.now().UTC()}
	if err := saveEdit(ctx, tx, edit); err != nil {
		return Order{}, err
	}
	updated := orderEvent{orderID: orderID, actorID: actorID, eventType: eventUpdated, payload: map[string]any{"changes": changes}}
	if err := writeEvent(ctx, tx, updated); err != nil {
		return Order{}, err
	}
	err = notify(ctx, tx, orderNotification{
		eventType: notifyOrderUpdated, groupID: groupID, orderID: orderID, amountUSD: next.AmountUSD,
		recipients: otherParties(actorID, next.GiverUserID, next.ReceiverUserID),
	}, edit.at)
	if err != nil {
		return Order{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("committing order edit: %w", err)
	}
	return s.Get(ctx, actorID, groupID, orderID)
}

func (in EditInput) empty() bool {
	return in.GiverUserID == nil && in.GiverCustomerPhone == nil &&
		in.ReceiverUserID == nil && in.ReceiverCustomerPhone == nil &&
		in.AmountUSD == nil && in.FeeUZS == nil
}

// apply returns the order's terms after the edit, validated like a new order.
func (o lockedOrder) apply(input EditInput) (CreateInput, error) {
	next := CreateInput{
		GiverUserID: o.parties.giverUserID, GiverCustomerPhone: o.giverPhone,
		ReceiverUserID: o.parties.receiverUserID, ReceiverCustomerPhone: o.receiverPhone,
		AmountUSD: o.amountUSD, FeeUZS: o.feeUZS,
	}
	if input.GiverUserID != nil {
		next.GiverUserID = *input.GiverUserID
	}
	if input.GiverCustomerPhone != nil {
		next.GiverCustomerPhone = *input.GiverCustomerPhone
	}
	if input.ReceiverUserID != nil {
		next.ReceiverUserID = *input.ReceiverUserID
	}
	if input.ReceiverCustomerPhone != nil {
		next.ReceiverCustomerPhone = *input.ReceiverCustomerPhone
	}
	if input.AmountUSD != nil {
		next.AmountUSD = *input.AmountUSD
	}
	if input.FeeUZS != nil {
		next.FeeUZS = *input.FeeUZS
	}
	return validCreateInput(next)
}

type change struct {
	Old any `json:"old"`
	New any `json:"new"`
}

func (o lockedOrder) changes(next CreateInput) map[string]change {
	changes := make(map[string]change)
	record := func(field string, old, new any) {
		if old != new {
			changes[field] = change{Old: old, New: new}
		}
	}
	record("giver_user_id", o.parties.giverUserID.String(), next.GiverUserID.String())
	record("receiver_user_id", o.parties.receiverUserID.String(), next.ReceiverUserID.String())
	record("giver_customer_phone", o.giverPhone, next.GiverCustomerPhone)
	record("receiver_customer_phone", o.receiverPhone, next.ReceiverCustomerPhone)
	record("amount_usd", o.amountUSD, next.AmountUSD)
	record("fee_uzs", o.feeUZS, next.FeeUZS)
	return changes
}

type orderEdit struct {
	groupID, orderID uuid.UUID
	current          lockedOrder
	next             CreateInput
	at               time.Time
}

// saveEdit writes the new terms and clears confirmations. A changed party gets
// their current location; an unchanged party keeps the original snapshot.
func saveEdit(ctx context.Context, q querier, e orderEdit) error {
	giver := partyRef{memberID: e.current.parties.giverMemberID, locationID: e.current.giverLocationID}
	receiver := partyRef{memberID: e.current.parties.receiverMemberID, locationID: e.current.receiverLocationID}
	var err error
	if e.next.GiverUserID != e.current.parties.giverUserID {
		if giver, err = resolveEmployee(ctx, q, e.groupID, e.next.GiverUserID); err != nil {
			return err
		}
	}
	if e.next.ReceiverUserID != e.current.parties.receiverUserID {
		if receiver, err = resolveEmployee(ctx, q, e.groupID, e.next.ReceiverUserID); err != nil {
			return err
		}
	}
	giverCustomerID, err := upsertCustomer(ctx, q, e.groupID, e.next.GiverCustomerPhone, e.at)
	if err != nil {
		return err
	}
	receiverCustomerID, err := upsertCustomer(ctx, q, e.groupID, e.next.ReceiverCustomerPhone, e.at)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `
		UPDATE orders
		SET giver_member_id = $3, receiver_member_id = $4,
		    giver_location_id = $5, receiver_location_id = $6,
		    giver_customer_id = $7, receiver_customer_id = $8,
		    amount_usd = $9, fee_uzs = $10, updated_at = $11
		WHERE id = $1 AND group_id = $2
	`, e.orderID, e.groupID, giver.memberID, receiver.memberID, giver.locationID, receiver.locationID,
		giverCustomerID, receiverCustomerID, e.next.AmountUSD, e.next.FeeUZS, e.at)
	if err != nil {
		return fmt.Errorf("updating order: %w", err)
	}
	_, err = q.Exec(ctx, `
		UPDATE order_confirmations
		SET deleted_at = $2, updated_at = $2
		WHERE order_id = $1 AND deleted_at IS NULL
	`, e.orderID, e.at)
	if err != nil {
		return fmt.Errorf("clearing order confirmations: %w", err)
	}
	return nil
}
