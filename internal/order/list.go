package order

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/paging"
)

// List returns the group's orders newest first. Employees only see orders
// they take part in.
func (s *Service) List(ctx context.Context, actorID, groupID uuid.UUID, input ListInput) ([]ListItem, error) {
	input, err := validListInput(input)
	if err != nil {
		return nil, err
	}
	v, err := loadViewer(ctx, s.pool, groupID, actorID)
	if err != nil {
		return nil, err
	}
	var onlyMember *uuid.UUID
	if v.isEmployee() {
		onlyMember = &v.memberID
	}
	rows, err := s.pool.Query(ctx, orderListQuery, groupID, input.Status, onlyMember, input.Limit, input.Offset)
	if err != nil {
		return nil, fmt.Errorf("listing orders: %w", err)
	}
	defer rows.Close()
	items := make([]ListItem, 0)
	for rows.Next() {
		item, err := scanListItem(rows, v)
		if err != nil {
			return nil, fmt.Errorf("scanning order: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating orders: %w", err)
	}
	return items, nil
}

func validListInput(input ListInput) (ListInput, error) {
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	switch input.Status {
	case "", StatusPending, StatusCompleted, StatusCancelled:
	default:
		return ListInput{}, apperror.New(apperror.ErrInvalidData, apperror.Text{UZ: "Status pending, completed yoki cancelled bo'lishi kerak", RU: "Статус должен быть pending, completed или cancelled", EN: "Status must be pending, completed or cancelled"})
	}
	page, err := paging.Page{Limit: input.Limit, Offset: input.Offset}.Valid()
	if err != nil {
		return ListInput{}, err
	}
	input.Limit, input.Offset = page.Limit, page.Offset
	return input, nil
}

var orderListQuery = fmt.Sprintf(`
	SELECT orders.id, orders.status, orders.amount_usd, orders.created_at,
	       orders.giver_member_id, giver_member.user_id, %s, COALESCE(giver_location.name, ''),
	       orders.receiver_member_id, receiver_member.user_id, %s, COALESCE(receiver_location.name, ''),
	       giver_confirmation.amount_usd, receiver_confirmation.amount_usd,
	       EXISTS (
	           SELECT 1 FROM order_cancellations cancellations
	           WHERE cancellations.order_id = orders.id
	             AND cancellations.status = 'pending'
	             AND cancellations.deleted_at IS NULL
	       )
	FROM orders
	JOIN group_members giver_member ON giver_member.id = orders.giver_member_id
	JOIN users giver_user ON giver_user.id = giver_member.user_id
	LEFT JOIN locations giver_location ON giver_location.id = orders.giver_location_id
	JOIN group_members receiver_member ON receiver_member.id = orders.receiver_member_id
	JOIN users receiver_user ON receiver_user.id = receiver_member.user_id
	LEFT JOIN locations receiver_location ON receiver_location.id = orders.receiver_location_id
	LEFT JOIN order_confirmations giver_confirmation
	  ON giver_confirmation.order_id = orders.id
	 AND giver_confirmation.member_id = orders.giver_member_id
	 AND giver_confirmation.deleted_at IS NULL
	LEFT JOIN order_confirmations receiver_confirmation
	  ON receiver_confirmation.order_id = orders.id
	 AND receiver_confirmation.member_id = orders.receiver_member_id
	 AND receiver_confirmation.deleted_at IS NULL
	WHERE orders.group_id = $1
	  AND orders.deleted_at IS NULL
	  AND ($2::text = '' OR orders.status = $2)
	  AND ($3::uuid IS NULL OR orders.giver_member_id = $3 OR orders.receiver_member_id = $3)
	ORDER BY orders.created_at DESC, orders.id DESC
	LIMIT $4 OFFSET $5
`, displayName("giver_user"), displayName("receiver_user"))

func scanListItem(row pgx.Row, v viewer) (ListItem, error) {
	var item ListItem
	var p parties
	var giverAmount, receiverAmount *int64
	var cancellationRequested bool
	err := row.Scan(
		&item.ID, &item.Status, &item.AmountUSD, &item.CreatedAt,
		&p.giverMemberID, &item.Giver.UserID, &item.Giver.Name, &item.Giver.LocationName,
		&p.receiverMemberID, &item.Receiver.UserID, &item.Receiver.Name, &item.Receiver.LocationName,
		&giverAmount, &receiverAmount, &cancellationRequested,
	)
	if err != nil {
		return ListItem{}, err
	}
	p.giverUserID, p.receiverUserID = item.Giver.UserID, item.Receiver.UserID
	var confirmations []confirmationRow
	if giverAmount != nil {
		confirmations = append(confirmations, confirmationRow{memberID: p.giverMemberID, amountUSD: *giverAmount})
	}
	if receiverAmount != nil {
		confirmations = append(confirmations, confirmationRow{memberID: p.receiverMemberID, amountUSD: *receiverAmount})
	}
	item.State, _ = present(item.Status, p, confirmations, v)
	if cancellationRequested {
		item.State = StateCancellationRequested
	}
	return item, nil
}
