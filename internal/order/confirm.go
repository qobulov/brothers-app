package order

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qobulov/brothers-app/pkg/apperror"
)

// Confirm records or corrects the actor's own confirmation. The order row is
// locked first, so confirmations and edits on one order run one at a time and
// completion effects apply exactly once.
func (s *Service) Confirm(ctx context.Context, actorID, groupID, orderID uuid.UUID, input ConfirmInput) (Order, error) {
	if err := validAmounts(input.AmountUSD, input.FeeUZS); err != nil {
		return Order{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("beginning order confirmation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	v, err := loadViewer(ctx, tx, groupID, actorID)
	if err != nil {
		return Order{}, err
	}
	locked, err := lockOrder(ctx, tx, groupID, orderID)
	if err != nil {
		return Order{}, err
	}
	if !v.canSee(locked.parties) {
		return Order{}, apperror.ErrRecordNotFound
	}
	if !v.isParty(locked.parties) {
		return Order{}, apperror.ErrForbidden
	}
	if locked.status != StatusPending {
		return Order{}, fmt.Errorf("%w: order is %s", apperror.ErrConflict, locked.status)
	}
	if err := requireNoOpenCancellation(ctx, tx, orderID); err != nil {
		return Order{}, err
	}

	now := s.now().UTC()
	eventType, err := saveConfirmation(ctx, tx, confirmationWrite{orderID: orderID, memberID: v.memberID, input: input, at: now})
	if err != nil {
		return Order{}, err
	}
	if err := writeEvent(ctx, tx, orderEvent{orderID: orderID, actorID: actorID, eventType: eventType}); err != nil {
		return Order{}, err
	}
	confirmations, err := loadConfirmations(ctx, tx, orderID)
	if err != nil {
		return Order{}, err
	}
	err = settle(ctx, tx, settlement{
		groupID: groupID, orderID: orderID, actorID: actorID,
		parties: locked.parties, confirmations: confirmations, at: now,
	})
	if err != nil {
		return Order{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("committing order confirmation: %w", err)
	}
	return s.Get(ctx, actorID, groupID, orderID)
}

// lockedOrder is the order's current state, read under a row lock.
type lockedOrder struct {
	status             string
	createdBy          uuid.UUID
	amountUSD, feeUZS  int64
	parties            parties
	giverLocationID    *uuid.UUID
	receiverLocationID *uuid.UUID
	giverPhone         string
	receiverPhone      string
}

func lockOrder(ctx context.Context, q querier, groupID, orderID uuid.UUID) (lockedOrder, error) {
	var o lockedOrder
	err := q.QueryRow(ctx, `
		SELECT orders.status, orders.created_by, orders.amount_usd, orders.fee_uzs,
		       orders.giver_member_id, giver_member.user_id, orders.giver_location_id, giver_customer.phone,
		       orders.receiver_member_id, receiver_member.user_id, orders.receiver_location_id, receiver_customer.phone
		FROM orders
		JOIN group_members giver_member ON giver_member.id = orders.giver_member_id
		JOIN customers giver_customer ON giver_customer.id = orders.giver_customer_id
		JOIN group_members receiver_member ON receiver_member.id = orders.receiver_member_id
		JOIN customers receiver_customer ON receiver_customer.id = orders.receiver_customer_id
		WHERE orders.id = $1 AND orders.group_id = $2 AND orders.deleted_at IS NULL
		FOR UPDATE OF orders
	`, orderID, groupID).Scan(
		&o.status, &o.createdBy, &o.amountUSD, &o.feeUZS,
		&o.parties.giverMemberID, &o.parties.giverUserID, &o.giverLocationID, &o.giverPhone,
		&o.parties.receiverMemberID, &o.parties.receiverUserID, &o.receiverLocationID, &o.receiverPhone,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return lockedOrder{}, apperror.ErrRecordNotFound
	}
	if err != nil {
		return lockedOrder{}, fmt.Errorf("locking order: %w", err)
	}
	return o, nil
}

type confirmationWrite struct {
	orderID, memberID uuid.UUID
	input             ConfirmInput
	at                time.Time
}

// saveConfirmation updates the active confirmation or inserts the first one,
// and reports which event that was.
func saveConfirmation(ctx context.Context, q querier, w confirmationWrite) (string, error) {
	result, err := q.Exec(ctx, `
		UPDATE order_confirmations
		SET amount_usd = $3, fee_uzs = $4, confirmed_at = $5, updated_at = $5
		WHERE order_id = $1 AND member_id = $2 AND deleted_at IS NULL
	`, w.orderID, w.memberID, w.input.AmountUSD, w.input.FeeUZS, w.at)
	if err != nil {
		return "", fmt.Errorf("correcting order confirmation: %w", err)
	}
	if result.RowsAffected() == 1 {
		return eventConfirmationCorrected, nil
	}
	_, err = q.Exec(ctx, `
		INSERT INTO order_confirmations (order_id, member_id, amount_usd, fee_uzs, confirmed_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5, $5)
	`, w.orderID, w.memberID, w.input.AmountUSD, w.input.FeeUZS, w.at)
	if err != nil {
		return "", fmt.Errorf("saving order confirmation: %w", err)
	}
	return eventConfirmed, nil
}

type settlement struct {
	groupID, orderID, actorID uuid.UUID
	parties                   parties
	confirmations             []confirmationRow
	at                        time.Time
}

// settle completes the order when both confirmations agree, or records the
// mismatch when they do not. With fewer than two confirmations it does nothing.
func settle(ctx context.Context, q querier, st settlement) error {
	if len(st.confirmations) < 2 {
		return nil
	}
	both := otherParties(uuid.Nil, st.parties.giverUserID, st.parties.receiverUserID)
	amount := st.confirmations[0].amountUSD
	if amount != st.confirmations[1].amountUSD {
		if err := writeEvent(ctx, q, orderEvent{orderID: st.orderID, actorID: st.actorID, eventType: eventAmountMismatch}); err != nil {
			return err
		}
		return notify(ctx, q, orderNotification{
			eventType: notifyOrderAmountMismatch, groupID: st.groupID, orderID: st.orderID, recipients: both,
		}, st.at)
	}
	if err := completeOrder(ctx, q, st, amount); err != nil {
		return err
	}
	completed := orderEvent{orderID: st.orderID, actorID: st.actorID, eventType: eventCompleted, payload: map[string]int64{"amount_usd": amount}}
	if err := writeEvent(ctx, q, completed); err != nil {
		return err
	}
	return notify(ctx, q, orderNotification{
		eventType: notifyOrderCompleted, groupID: st.groupID, orderID: st.orderID, amountUSD: amount, recipients: both,
	}, st.at)
}
