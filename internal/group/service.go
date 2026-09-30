package group

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

type Group struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	IsOwner   bool      `json:"is_owner"`
	CreatedAt time.Time `json:"-"`
}

// GroupListItem contains the summary fields returned only by the group list.
type GroupListItem struct {
	Group
	GroupBalanceUSD    int64  `json:"group_balance_usd"`
	MyProfitUZS        *int64 `json:"my_profit_uzs,omitempty"`
	MembersCount       int64  `json:"members_count"`
	LocationsCount     int64  `json:"locations_count"`
	CustomersCount     int64  `json:"customers_count"`
	OrderCount         int64  `json:"order_count"`
	SubscriptionActive bool   `json:"subscription_active"`
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, now: time.Now}
}

func (s *Service) Create(ctx context.Context, actorID uuid.UUID, name string) (Group, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 255 {
		return Group{}, apperror.New(apperror.ErrInvalidData, apperror.Text{UZ: "Guruh nomi 1 dan 255 belgigacha bo'lishi kerak", RU: "Название группы должно содержать от 1 до 255 символов", EN: "The group name must be 1-255 characters"})
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Group{}, fmt.Errorf("beginning group creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	groupID := uuid.New()
	now := s.now().UTC()
	_, err = tx.Exec(ctx, `
		INSERT INTO groups (id, name, created_by, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, true, $4, $4)
	`, groupID, name, actorID, now)
	if err != nil {
		return Group{}, fmt.Errorf("creating group: %w", err)
	}
	result, err := tx.Exec(ctx, `
		INSERT INTO group_members (id, group_id, user_id, username, role, is_owner, joined_at, created_at, updated_at)
		SELECT $1, $2, id, username, 'manager'::user_role, true, $3, $3, $3
		FROM users
		WHERE id = $4 AND deleted_at IS NULL
	`, uuid.New(), groupID, now, actorID)
	if err != nil {
		return Group{}, fmt.Errorf("creating owner membership: %w", err)
	}
	// A session can outlive its account; never commit a group without an owner.
	if result.RowsAffected() != 1 {
		return Group{}, apperror.ErrUnauthorized
	}
	if err := tx.Commit(ctx); err != nil {
		return Group{}, fmt.Errorf("committing group creation: %w", err)
	}
	return Group{ID: groupID, Name: name, Role: "manager", IsOwner: true, CreatedAt: now}, nil
}

func (s *Service) List(ctx context.Context, actorID uuid.UUID) ([]GroupListItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT groups.id, groups.name, group_members.role::text, group_members.is_owner,
		       COALESCE((
		           SELECT SUM(balances.balance_usd)::bigint
		           FROM employee_balances balances
		           JOIN group_members balance_members
		             ON balance_members.group_id = balances.group_id
		            AND balance_members.id = balances.member_id
		           WHERE balances.group_id = groups.id
		             AND balances.deleted_at IS NULL
		             AND balance_members.deleted_at IS NULL
		             AND balance_members.role::text = 'employee'
		             AND (
		                 balances.member_id = group_members.id
		                 OR group_members.is_owner
		                 OR group_members.role::text IN ('owner', 'admin', 'manager', 'investor')
		             )
		       ), 0)::bigint AS group_balance_usd,
		       -- Employees see their own all-time profit; owner, manager and
		       -- investor see the whole group's, including members who left.
		       CASE
		           WHEN group_members.role::text = 'employee' THEN COALESCE((
		               SELECT SUM(profits.profit_uzs)::bigint
		               FROM member_profit_periods profits
		               WHERE profits.group_id = groups.id
		                 AND profits.member_id = group_members.id
		                 AND profits.deleted_at IS NULL
		           ), 0)::bigint
		           WHEN group_members.is_owner
		             OR group_members.role::text IN ('owner', 'admin', 'manager', 'investor') THEN COALESCE((
		               SELECT SUM(profits.profit_uzs)::bigint
		               FROM member_profit_periods profits
		               WHERE profits.group_id = groups.id
		                 AND profits.deleted_at IS NULL
		           ), 0)::bigint
		       END AS my_profit_uzs,
		       (
		           SELECT COUNT(*)::bigint
		           FROM group_members members
		           WHERE members.group_id = groups.id
		             AND members.deleted_at IS NULL
		       ) AS members_count,
		       (
		           SELECT COUNT(*)::bigint
		           FROM locations
		           WHERE locations.group_id = groups.id
		             AND locations.deleted_at IS NULL
		       ) AS locations_count,
		       (
		           SELECT COUNT(*)::bigint
		           FROM customers
		           WHERE customers.group_id = groups.id
		             AND customers.deleted_at IS NULL
		       ) AS customers_count,
		       (
		           SELECT COUNT(*)::bigint
		           FROM orders
		           WHERE orders.group_id = groups.id
		             AND orders.deleted_at IS NULL
		       ) AS order_count,
		       true AS subscription_active,
		       groups.created_at
		FROM group_members
		JOIN groups ON groups.id = group_members.group_id
		WHERE group_members.user_id = $1
		  AND group_members.deleted_at IS NULL
		  AND groups.deleted_at IS NULL
		  AND groups.is_active
		ORDER BY groups.created_at DESC
	`, actorID)
	if err != nil {
		return nil, fmt.Errorf("listing groups: %w", err)
	}
	defer rows.Close()

	groups := make([]GroupListItem, 0)
	for rows.Next() {
		var item GroupListItem
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Role, &item.IsOwner,
			&item.GroupBalanceUSD, &item.MyProfitUZS,
			&item.MembersCount, &item.LocationsCount, &item.CustomersCount,
			&item.OrderCount, &item.SubscriptionActive,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning group: %w", err)
		}
		groups = append(groups, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating groups: %w", err)
	}
	return groups, nil
}

// Delete soft-deletes a group and revokes normal application access while
// preserving its financial and audit history.
func (s *Service) Delete(ctx context.Context, actorID, groupID uuid.UUID, confirmed bool) error {
	if !confirmed {
		return apperror.New(apperror.ErrInvalidData, apperror.Text{UZ: "Guruhni o'chirishni tasdiqlang (confirm: true)", RU: "Подтвердите удаление группы (confirm: true)", EN: "Confirm the deletion with confirm: true"})
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning group deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var groupName string
	var isOwner bool
	err = tx.QueryRow(ctx, `
		SELECT groups.name,
		       EXISTS (
		           SELECT 1
		           FROM group_members
		           WHERE group_members.group_id = groups.id
		             AND group_members.user_id = $2
		             AND group_members.is_owner
		             AND group_members.deleted_at IS NULL
		       ) AS is_owner
		FROM groups
		WHERE groups.id = $1
		  AND groups.deleted_at IS NULL
		  AND groups.is_active
		FOR UPDATE
	`, groupID, actorID).Scan(&groupName, &isOwner)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperror.ErrRecordNotFound
	}
	if err != nil {
		return fmt.Errorf("locking group for deletion: %w", err)
	}
	if !isOwner {
		return apperror.ErrForbidden
	}

	now := s.now().UTC()
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_logs (
			id, group_id, actor_user_id, action, entity_type, entity_id,
			old_data, new_data, created_at, updated_at
		)
		VALUES (
			$1, $2, $3, 'group.deleted', 'group', $2,
			jsonb_build_object('name', $4::text, 'is_active', true),
			jsonb_build_object('is_active', false, 'deleted_at', $5::timestamptz),
			$5, $5
		)
	`, uuid.New(), groupID, actorID, groupName, now)
	if err != nil {
		return fmt.Errorf("writing group deletion audit log: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE group_invitations
		SET status = CASE WHEN status = 'pending' THEN 'revoked' ELSE status END,
		    revoked_at = CASE WHEN status = 'pending' THEN $2 ELSE revoked_at END,
		    responded_at = CASE WHEN status = 'pending' THEN COALESCE(responded_at, $2) ELSE responded_at END,
		    updated_at = $2,
		    deleted_at = $2
		WHERE group_id = $1 AND deleted_at IS NULL
	`, groupID, now)
	if err != nil {
		return fmt.Errorf("revoking group invitations: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE notification_recipients recipients
		SET is_read = true,
		    read_at = COALESCE(recipients.read_at, $2),
		    updated_at = $2
		FROM notifications
		WHERE recipients.notification_id = notifications.id
		  AND notifications.payload->>'event_type' = 'GROUP_INVITATION'
		  AND notifications.payload->>'group_id' = $1::text
		  AND recipients.is_read = false
		  AND recipients.deleted_at IS NULL
		  AND notifications.deleted_at IS NULL
	`, groupID, now)
	if err != nil {
		return fmt.Errorf("closing group invitation notifications: %w", err)
	}

	for _, operation := range []struct {
		name  string
		query string
	}{
		{
			name: "group locations",
			query: `UPDATE locations
			        SET deleted_at = $2, updated_at = $2
			        WHERE group_id = $1 AND deleted_at IS NULL`,
		},
		{
			name: "group customers",
			query: `UPDATE customers
			        SET deleted_at = $2, updated_at = $2
			        WHERE group_id = $1 AND deleted_at IS NULL`,
		},
		{
			name: "group orders",
			query: `UPDATE orders
			        SET deleted_at = $2, updated_at = $2
			        WHERE group_id = $1 AND deleted_at IS NULL`,
		},
		{
			name: "group members",
			query: `UPDATE group_members
			        SET deleted_at = $2, updated_at = $2
			        WHERE group_id = $1 AND deleted_at IS NULL`,
		},
	} {
		if _, err := tx.Exec(ctx, operation.query, groupID, now); err != nil {
			return fmt.Errorf("deleting %s: %w", operation.name, err)
		}
	}

	result, err := tx.Exec(ctx, `
		UPDATE groups
		SET is_active = false, deleted_at = $2, updated_at = $2
		WHERE id = $1 AND deleted_at IS NULL AND is_active
	`, groupID, now)
	if err != nil {
		return fmt.Errorf("deleting group: %w", err)
	}
	if result.RowsAffected() != 1 {
		return apperror.ErrRecordNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing group deletion: %w", err)
	}
	return nil
}
