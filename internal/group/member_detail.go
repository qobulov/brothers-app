package group

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/qobulov/brothers-app/pkg/apperror"
)

type MemberLocation struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// RemovalState describes the member, not the viewer: what still blocks
// removing them from the group.
type RemovalState struct {
	Allowed        bool `json:"allowed"`
	BalanceIsZero  bool `json:"balance_is_zero"`
	NoActiveOrders bool `json:"no_active_orders"`
}

// MemberPermissions describes what the viewer may attempt on this member.
type MemberPermissions struct {
	CanEditRole      bool `json:"can_edit_role"`
	CanEditLocation  bool `json:"can_edit_location"`
	CanAdjustBalance bool `json:"can_adjust_balance"`
	CanRemove        bool `json:"can_remove"`
}

type MemberDetail struct {
	MemberID    uuid.UUID         `json:"member_id"`
	UserID      uuid.UUID         `json:"user_id"`
	FullName    string            `json:"full_name"`
	Username    string            `json:"username"`
	Email       string            `json:"email"`
	AvatarURL   string            `json:"avatar_url"`
	Role        string            `json:"role" enums:"employee,manager,investor"`
	IsOwner     bool              `json:"is_owner"`
	Location    *MemberLocation   `json:"location"`
	BalanceUSD  *int64            `json:"balance_usd,omitempty"`
	ProfitUZS   *int64            `json:"profit_uzs,omitempty"`
	JoinedAt    time.Time         `json:"joined_at"`
	Removal     RemovalState      `json:"removal"`
	Permissions MemberPermissions `json:"permissions"`
}

// GetMember returns one member. Employees can only open their own page.
func (s *Service) GetMember(ctx context.Context, actorID, groupID, userID uuid.UUID) (MemberDetail, error) {
	a, err := loadActor(ctx, s.pool, groupID, actorID)
	if err != nil {
		return MemberDetail{}, err
	}
	target, err := loadMember(ctx, s.pool, memberLookup{groupID: groupID, userID: userID})
	if err != nil {
		return MemberDetail{}, err
	}
	if !a.canView(target) {
		return MemberDetail{}, apperror.ErrRecordNotFound
	}
	return target.detail(a), nil
}

// querier is satisfied by both *pgxpool.Pool and pgx.Tx.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// actor is the requesting user's active membership in the group.
type actor struct {
	userID   uuid.UUID
	memberID uuid.UUID
	role     string
	isOwner  bool
}

func (a actor) isManager() bool {
	return a.isOwner || a.role == "manager" || a.role == "owner" || a.role == "admin"
}

// canView lets owner, managers and investors see anyone; everyone else only themselves.
func (a actor) canView(target memberRow) bool {
	return a.isManager() || a.role == "investor" || a.memberID == target.memberID
}

// canRemove is the role rule only; the member's balance and orders are checked separately.
func (a actor) canRemove(target memberRow) bool {
	if target.isOwner || a.memberID == target.memberID {
		return false
	}
	if target.role == roleEmployee {
		return a.isManager()
	}
	return a.isOwner
}

func loadActor(ctx context.Context, q querier, groupID, userID uuid.UUID) (actor, error) {
	a := actor{userID: userID}
	err := q.QueryRow(ctx, `
		SELECT members.id, members.role::text, members.is_owner
		FROM group_members members
		JOIN groups ON groups.id = members.group_id
		WHERE members.group_id = $1
		  AND members.user_id = $2
		  AND members.deleted_at IS NULL
		  AND groups.deleted_at IS NULL
		  AND groups.is_active
	`, groupID, userID).Scan(&a.memberID, &a.role, &a.isOwner)
	if errors.Is(err, pgx.ErrNoRows) {
		return actor{}, apperror.ErrRecordNotFound
	}
	if err != nil {
		return actor{}, fmt.Errorf("loading group membership: %w", err)
	}
	return a, nil
}

const roleEmployee = "employee"

// memberRow is one active member with the numbers that decide what may be done to them.
type memberRow struct {
	memberID     uuid.UUID
	userID       uuid.UUID
	fullName     string
	username     string
	email        string
	avatarURL    string
	role         string
	isOwner      bool
	joinedAt     time.Time
	location     *MemberLocation
	balanceUSD   int64
	profitUZS    int64
	activeOrders int
}

func (m memberRow) removal() RemovalState {
	state := RemovalState{BalanceIsZero: m.balanceUSD == 0, NoActiveOrders: m.activeOrders == 0}
	state.Allowed = state.BalanceIsZero && state.NoActiveOrders
	return state
}

func (m memberRow) detail(a actor) MemberDetail {
	d := MemberDetail{
		MemberID: m.memberID, UserID: m.userID, FullName: m.fullName, Username: m.username,
		Email: m.email, AvatarURL: m.avatarURL, Role: m.role, IsOwner: m.isOwner,
		Location: m.location, JoinedAt: m.joinedAt, Removal: m.removal(),
	}
	employee := m.role == roleEmployee
	if employee {
		balance, profit := m.balanceUSD, m.profitUZS
		d.BalanceUSD, d.ProfitUZS = &balance, &profit
	}
	d.Permissions = MemberPermissions{
		CanEditRole:      a.isOwner && !m.isOwner,
		CanEditLocation:  a.isManager() && employee,
		CanAdjustBalance: a.isManager() && employee,
		CanRemove:        a.canRemove(m),
	}
	return d
}

type memberLookup struct {
	groupID, userID uuid.UUID
	// lock takes a row lock on the membership for the rest of the transaction.
	lock bool
}

// An order is active for a member while it is pending, or while it has an
// open cancellation request, and the member is its giver or receiver.
const memberQuery = `
	SELECT members.id, members.user_id,
	       COALESCE(
	           NULLIF(btrim(concat_ws(' ', users.first_name, users.last_name)), ''),
	           NULLIF(users.name, ''), NULLIF(users.username, ''), ''
	       ),
	       COALESCE(users.username, ''), COALESCE(users.email, ''), COALESCE(users.avatar_url, ''),
	       members.role::text, members.is_owner, members.joined_at,
	       locations.id, locations.name,
	       COALESCE((
	           SELECT SUM(balances.balance_usd)
	           FROM employee_balances balances
	           WHERE balances.group_id = members.group_id
	             AND balances.member_id = members.id
	             AND balances.deleted_at IS NULL
	       ), 0)::bigint,
	       COALESCE((
	           SELECT SUM(profits.profit_uzs)
	           FROM member_profit_periods profits
	           WHERE profits.group_id = members.group_id
	             AND profits.member_id = members.id
	             AND profits.deleted_at IS NULL
	       ), 0)::bigint,
	       (
	           SELECT COUNT(*)
	           FROM orders
	           WHERE orders.group_id = members.group_id
	             AND orders.deleted_at IS NULL
	             AND members.id IN (orders.giver_member_id, orders.receiver_member_id)
	             AND (
	                 orders.status = 'pending'
	                 OR EXISTS (
	                     SELECT 1 FROM order_cancellations cancellations
	                     WHERE cancellations.order_id = orders.id
	                       AND cancellations.status = 'pending'
	                       AND cancellations.deleted_at IS NULL
	                 )
	             )
	       )
	FROM group_members members
	JOIN users ON users.id = members.user_id
	LEFT JOIN locations
	  ON locations.group_id = members.group_id
	 AND locations.employee_id = members.id
	 AND locations.deleted_at IS NULL
	WHERE members.group_id = $1 AND members.user_id = $2 AND members.deleted_at IS NULL
`

func loadMember(ctx context.Context, q querier, lookup memberLookup) (memberRow, error) {
	query := memberQuery
	if lookup.lock {
		query += " FOR UPDATE OF members"
	}
	var m memberRow
	var locationID *uuid.UUID
	var locationName *string
	err := q.QueryRow(ctx, query, lookup.groupID, lookup.userID).Scan(
		&m.memberID, &m.userID, &m.fullName, &m.username, &m.email, &m.avatarURL,
		&m.role, &m.isOwner, &m.joinedAt, &locationID, &locationName,
		&m.balanceUSD, &m.profitUZS, &m.activeOrders,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return memberRow{}, apperror.ErrRecordNotFound
	}
	if err != nil {
		return memberRow{}, fmt.Errorf("loading group member: %w", err)
	}
	if locationID != nil && locationName != nil {
		m.location = &MemberLocation{ID: *locationID, Name: *locationName}
	}
	return m, nil
}
