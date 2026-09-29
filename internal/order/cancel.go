package order

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qobulov/brothers-app/pkg/apperror"
)

const (
	cancellationPending  = "pending"
	cancellationApproved = "approved"
	cancellationRejected = "rejected"

	CancellationApprove = "approve"
	CancellationReject  = "reject"

	maxCancellationReason = 500
)

// RequestCancellation opens a cancellation request. The requesting party
// counts as approved; the order is cancelled once the other party approves.
func (s *Service) RequestCancellation(ctx context.Context, actorID, groupID, orderID uuid.UUID, reason string) (Order, error) {
	reason = strings.TrimSpace(reason)
	if utf8.RuneCountInString(reason) > maxCancellationReason {
		return Order{}, fmt.Errorf("%w: reason must be at most %d characters", apperror.ErrInvalidData, maxCancellationReason)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("beginning cancellation request: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	v, locked, err := lockForParty(ctx, tx, actorID, groupID, orderID)
	if err != nil {
		return Order{}, err
	}
	if locked.status == StatusCancelled {
		return Order{}, fmt.Errorf("%w: order is already cancelled", apperror.ErrConflict)
	}
	if err := requireNoOpenCancellation(ctx, tx, orderID); err != nil {
		return Order{}, err
	}
	now := s.now().UTC()
	_, err = tx.Exec(ctx, `
		INSERT INTO order_cancellations (order_id, requested_by_member_id, reason, status, created_at, updated_at)
		VALUES ($1, $2, NULLIF($3, ''), 'pending', $4, $4)
	`, orderID, v.memberID, reason, now)
	if err != nil {
		return Order{}, fmt.Errorf("creating cancellation request: %w", err)
	}
	if err := writeEvent(ctx, tx, orderEvent{orderID: orderID, actorID: actorID, eventType: eventCancellationRequested}); err != nil {
		return Order{}, err
	}
	err = notify(ctx, tx, orderNotification{
		eventType: notifyOrderCancellationRequested, groupID: groupID, orderID: orderID, amountUSD: locked.amountUSD,
		recipients: otherParties(actorID, locked.parties.giverUserID, locked.parties.receiverUserID),
	}, now)
	if err != nil {
		return Order{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("committing cancellation request: %w", err)
	}
	return s.Get(ctx, actorID, groupID, orderID)
}

// RespondCancellation approves or rejects the open request. Only the other
// party can approve; either party can reject, which lets the requester
// withdraw. Approving a completed order reverses its money effects.
func (s *Service) RespondCancellation(ctx context.Context, actorID, groupID, orderID uuid.UUID, action string) (Order, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	if action != CancellationApprove && action != CancellationReject {
		return Order{}, fmt.Errorf("%w: action must be approve or reject", apperror.ErrInvalidData)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("beginning cancellation response: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	v, locked, err := lockForParty(ctx, tx, actorID, groupID, orderID)
	if err != nil {
		return Order{}, err
	}
	request, err := lockOpenCancellation(ctx, tx, orderID)
	if err != nil {
		return Order{}, err
	}
	response := cancellationResponse{
		groupID: groupID, orderID: orderID, actorID: actorID, responderID: v.memberID,
		requestID: request.id, locked: locked, at: s.now().UTC(),
	}
	if action == CancellationReject {
		err = rejectCancellation(ctx, tx, response)
	} else if request.requesterID == v.memberID {
		err = fmt.Errorf("%w: the other employee must approve the cancellation", apperror.ErrForbidden)
	} else {
		err = approveCancellation(ctx, tx, response)
	}
	if err != nil {
		return Order{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("committing cancellation response: %w", err)
	}
	return s.Get(ctx, actorID, groupID, orderID)
}

// lockForParty locks the order and requires the actor to be its giver or receiver.
func lockForParty(ctx context.Context, q querier, actorID, groupID, orderID uuid.UUID) (viewer, lockedOrder, error) {
	v, err := loadViewer(ctx, q, groupID, actorID)
	if err != nil {
		return viewer{}, lockedOrder{}, err
	}
	locked, err := lockOrder(ctx, q, groupID, orderID)
	if err != nil {
		return viewer{}, lockedOrder{}, err
	}
	if !v.canSee(locked.parties) {
		return viewer{}, lockedOrder{}, apperror.ErrRecordNotFound
	}
	if !v.isParty(locked.parties) {
		return viewer{}, lockedOrder{}, apperror.ErrForbidden
	}
	return v, locked, nil
}

func requireNoOpenCancellation(ctx context.Context, q querier, orderID uuid.UUID) error {
	var open bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM order_cancellations
			WHERE order_id = $1 AND status = 'pending' AND deleted_at IS NULL
		)
	`, orderID).Scan(&open)
	if err != nil {
		return fmt.Errorf("checking cancellation request: %w", err)
	}
	if open {
		return fmt.Errorf("%w: order has an open cancellation request", apperror.ErrConflict)
	}
	return nil
}

type openCancellation struct {
	id          uuid.UUID
	requesterID uuid.UUID
}

func lockOpenCancellation(ctx context.Context, q querier, orderID uuid.UUID) (openCancellation, error) {
	var request openCancellation
	err := q.QueryRow(ctx, `
		SELECT id, requested_by_member_id
		FROM order_cancellations
		WHERE order_id = $1 AND status = 'pending' AND deleted_at IS NULL
		FOR UPDATE
	`, orderID).Scan(&request.id, &request.requesterID)
	if errors.Is(err, pgx.ErrNoRows) {
		return openCancellation{}, fmt.Errorf("%w: order has no open cancellation request", apperror.ErrConflict)
	}
	if err != nil {
		return openCancellation{}, fmt.Errorf("locking cancellation request: %w", err)
	}
	return request, nil
}

type cancellationResponse struct {
	groupID, orderID, actorID uuid.UUID
	responderID, requestID    uuid.UUID
	locked                    lockedOrder
	at                        time.Time
}

func (r cancellationResponse) close(ctx context.Context, q querier, status string) error {
	_, err := q.Exec(ctx, `
		UPDATE order_cancellations
		SET status = $2, responded_by_member_id = $3, responded_at = $4, updated_at = $4
		WHERE id = $1
	`, r.requestID, status, r.responderID, r.at)
	if err != nil {
		return fmt.Errorf("closing cancellation request: %w", err)
	}
	return nil
}

func rejectCancellation(ctx context.Context, q querier, r cancellationResponse) error {
	if err := r.close(ctx, q, cancellationRejected); err != nil {
		return err
	}
	if err := writeEvent(ctx, q, orderEvent{orderID: r.orderID, actorID: r.actorID, eventType: eventCancellationRejected}); err != nil {
		return err
	}
	return notify(ctx, q, orderNotification{
		eventType: notifyOrderCancellationRejected, groupID: r.groupID, orderID: r.orderID, amountUSD: r.locked.amountUSD,
		recipients: otherParties(r.actorID, r.locked.parties.giverUserID, r.locked.parties.receiverUserID),
	}, r.at)
}

func approveCancellation(ctx context.Context, q querier, r cancellationResponse) error {
	if err := r.close(ctx, q, cancellationApproved); err != nil {
		return err
	}
	reversed := r.locked.status == StatusCompleted
	if reversed {
		confirmations, err := loadConfirmations(ctx, q, r.orderID)
		if err != nil {
			return err
		}
		err = reverseCompletion(ctx, q, settlement{
			groupID: r.groupID, orderID: r.orderID, actorID: r.actorID,
			parties: r.locked.parties, confirmations: confirmations, at: r.at,
		})
		if err != nil {
			return err
		}
	}
	_, err := q.Exec(ctx, `
		UPDATE orders SET status = 'cancelled', cancelled_at = $2, updated_at = $2 WHERE id = $1
	`, r.orderID, r.at)
	if err != nil {
		return fmt.Errorf("cancelling order: %w", err)
	}
	cancelled := orderEvent{orderID: r.orderID, actorID: r.actorID, eventType: eventCancelled, payload: map[string]bool{"reversed": reversed}}
	if err := writeEvent(ctx, q, cancelled); err != nil {
		return err
	}
	return notify(ctx, q, orderNotification{
		eventType: notifyOrderCancelled, groupID: r.groupID, orderID: r.orderID, amountUSD: r.locked.amountUSD,
		recipients: otherParties(uuid.Nil, r.locked.parties.giverUserID, r.locked.parties.receiverUserID),
	}, r.at)
}

// loadCancellation returns the open request, or the approved one for a
// cancelled order, with each party's approval.
func loadCancellation(ctx context.Context, q querier, orderID uuid.UUID, p parties) (*Cancellation, error) {
	var c Cancellation
	var requesterMemberID uuid.UUID
	var reason *string
	err := q.QueryRow(ctx, cancellationQuery, orderID).Scan(
		&c.ID, &c.Status, &reason, &c.RequestedAt, &c.RespondedAt,
		&requesterMemberID, &c.RequestedBy.UserID, &c.RequestedBy.Name,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("loading cancellation request: %w", err)
	}
	if reason != nil {
		c.Reason = *reason
	}
	approval := func(role string, memberID, userID uuid.UUID) Approval {
		status := "waiting"
		if memberID == requesterMemberID || c.Status == cancellationApproved {
			status = "approved"
		}
		return Approval{Role: role, UserID: userID, Status: status}
	}
	c.Approvals = []Approval{
		approval(RoleGiver, p.giverMemberID, p.giverUserID),
		approval(RoleReceiver, p.receiverMemberID, p.receiverUserID),
	}
	return &c, nil
}

var cancellationQuery = fmt.Sprintf(`
	SELECT cancellations.id, cancellations.status, cancellations.reason,
	       cancellations.created_at, cancellations.responded_at,
	       cancellations.requested_by_member_id, requester.user_id, %s
	FROM order_cancellations cancellations
	JOIN group_members requester ON requester.id = cancellations.requested_by_member_id
	JOIN users ON users.id = requester.user_id
	WHERE cancellations.order_id = $1
	  AND cancellations.status IN ('pending', 'approved')
	  AND cancellations.deleted_at IS NULL
	ORDER BY cancellations.created_at DESC
	LIMIT 1
`, displayName("users"))
