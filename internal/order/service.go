package order

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qobulov/brothers-app/pkg/apperror"
)

type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, now: time.Now}
}

func (s *Service) Create(ctx context.Context, actorID, groupID uuid.UUID, input CreateInput) (Order, error) {
	input, err := validCreateInput(input)
	if err != nil {
		return Order{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("beginning order creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	actor, err := loadViewer(ctx, tx, groupID, actorID)
	if err != nil {
		return Order{}, err
	}
	if err := actor.canWrite(input.GiverUserID, input.ReceiverUserID); err != nil {
		return Order{}, err
	}
	giver, err := resolveEmployee(ctx, tx, groupID, input.GiverUserID)
	if err != nil {
		return Order{}, err
	}
	receiver, err := resolveEmployee(ctx, tx, groupID, input.ReceiverUserID)
	if err != nil {
		return Order{}, err
	}
	now := s.now().UTC()
	giverCustomerID, err := upsertCustomer(ctx, tx, groupID, input.GiverCustomerPhone, now)
	if err != nil {
		return Order{}, err
	}
	receiverCustomerID, err := upsertCustomer(ctx, tx, groupID, input.ReceiverCustomerPhone, now)
	if err != nil {
		return Order{}, err
	}

	orderID := uuid.New()
	_, err = tx.Exec(ctx, `
		INSERT INTO orders (
			id, group_id, created_by,
			giver_member_id, receiver_member_id, giver_location_id, receiver_location_id,
			giver_customer_id, receiver_customer_id, amount_usd, fee_uzs,
			status, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'pending', $12, $12)
	`, orderID, groupID, actorID,
		giver.memberID, receiver.memberID, giver.locationID, receiver.locationID,
		giverCustomerID, receiverCustomerID, input.AmountUSD, input.FeeUZS, now)
	if err != nil {
		return Order{}, fmt.Errorf("creating order: %w", err)
	}
	if err := writeEvent(ctx, tx, orderEvent{orderID: orderID, actorID: actorID, eventType: eventCreated}); err != nil {
		return Order{}, err
	}
	err = notify(ctx, tx, orderNotification{
		eventType: notifyOrderCreated, groupID: groupID, orderID: orderID, amountUSD: input.AmountUSD,
		recipients: otherParties(actorID, input.GiverUserID, input.ReceiverUserID),
	}, now)
	if err != nil {
		return Order{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("committing order creation: %w", err)
	}
	return s.Get(ctx, actorID, groupID, orderID)
}

func (s *Service) Get(ctx context.Context, actorID, groupID, orderID uuid.UUID) (Order, error) {
	v, err := loadViewer(ctx, s.pool, groupID, actorID)
	if err != nil {
		return Order{}, err
	}
	result, p, err := loadOrder(ctx, s.pool, groupID, orderID)
	if err != nil {
		return Order{}, err
	}
	if !v.canSee(p) {
		return Order{}, apperror.ErrRecordNotFound
	}
	rows, err := loadConfirmations(ctx, s.pool, orderID)
	if err != nil {
		return Order{}, err
	}
	result.State, result.Confirmations = present(result.Status, p, rows, v)
	if result.Cancellation, err = loadCancellation(ctx, s.pool, orderID, p); err != nil {
		return Order{}, err
	}
	if result.Cancellation != nil && result.Cancellation.Status == cancellationPending {
		result.State = StateCancellationRequested
	}
	return result, nil
}

// Locations are joined without a deleted_at filter: the order keeps the
// location it was created with even after that location is removed.
var orderDetailQuery = fmt.Sprintf(`
	SELECT orders.id, orders.group_id, orders.status, orders.amount_usd, orders.fee_uzs,
	       orders.giver_member_id, giver_member.user_id, %s, COALESCE(giver_user.avatar_url, ''),
	       giver_location.id, giver_location.name, giver_customer.phone,
	       orders.receiver_member_id, receiver_member.user_id, %s, COALESCE(receiver_user.avatar_url, ''),
	       receiver_location.id, receiver_location.name, receiver_customer.phone,
	       orders.created_by, %s,
	       orders.created_at, orders.updated_at, orders.completed_at
	FROM orders
	JOIN group_members giver_member ON giver_member.id = orders.giver_member_id
	JOIN users giver_user ON giver_user.id = giver_member.user_id
	LEFT JOIN locations giver_location ON giver_location.id = orders.giver_location_id
	JOIN customers giver_customer ON giver_customer.id = orders.giver_customer_id
	JOIN group_members receiver_member ON receiver_member.id = orders.receiver_member_id
	JOIN users receiver_user ON receiver_user.id = receiver_member.user_id
	LEFT JOIN locations receiver_location ON receiver_location.id = orders.receiver_location_id
	JOIN customers receiver_customer ON receiver_customer.id = orders.receiver_customer_id
	JOIN users creator ON creator.id = orders.created_by
	WHERE orders.id = $1 AND orders.group_id = $2 AND orders.deleted_at IS NULL
`, displayName("giver_user"), displayName("receiver_user"), displayName("creator"))

func loadOrder(ctx context.Context, q querier, groupID, orderID uuid.UUID) (Order, parties, error) {
	var o Order
	var p parties
	var giverLocationID, receiverLocationID *uuid.UUID
	var giverLocationName, receiverLocationName *string
	err := q.QueryRow(ctx, orderDetailQuery, orderID, groupID).Scan(
		&o.ID, &o.GroupID, &o.Status, &o.AmountUSD, &o.FeeUZS,
		&p.giverMemberID, &o.Giver.UserID, &o.Giver.Name, &o.Giver.AvatarURL,
		&giverLocationID, &giverLocationName, &o.Giver.CustomerPhone,
		&p.receiverMemberID, &o.Receiver.UserID, &o.Receiver.Name, &o.Receiver.AvatarURL,
		&receiverLocationID, &receiverLocationName, &o.Receiver.CustomerPhone,
		&o.CreatedBy.UserID, &o.CreatedBy.Name,
		&o.CreatedAt, &o.UpdatedAt, &o.CompletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, parties{}, apperror.ErrRecordNotFound
	}
	if err != nil {
		return Order{}, parties{}, fmt.Errorf("loading order: %w", err)
	}
	p.giverUserID, p.receiverUserID = o.Giver.UserID, o.Receiver.UserID
	o.Giver.Location = optionalLocation(giverLocationID, giverLocationName)
	o.Receiver.Location = optionalLocation(receiverLocationID, receiverLocationName)
	return o, p, nil
}

func optionalLocation(id *uuid.UUID, name *string) *Location {
	if id == nil || name == nil {
		return nil
	}
	return &Location{ID: *id, Name: *name}
}

func loadConfirmations(ctx context.Context, q querier, orderID uuid.UUID) ([]confirmationRow, error) {
	rows, err := q.Query(ctx, `
		SELECT member_id, amount_usd, fee_uzs, confirmed_at
		FROM order_confirmations
		WHERE order_id = $1 AND deleted_at IS NULL
	`, orderID)
	if err != nil {
		return nil, fmt.Errorf("loading order confirmations: %w", err)
	}
	defer rows.Close()
	var result []confirmationRow
	for rows.Next() {
		var row confirmationRow
		if err := rows.Scan(&row.memberID, &row.amountUSD, &row.feeUZS, &row.confirmedAt); err != nil {
			return nil, fmt.Errorf("scanning order confirmation: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating order confirmations: %w", err)
	}
	return result, nil
}
