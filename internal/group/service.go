package group

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/helpers"
)

const invitationLifetime = 7 * 24 * time.Hour

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

type Invitation struct {
	ID           uuid.UUID  `json:"id"`
	GroupID      uuid.UUID  `json:"group_id"`
	GroupName    string     `json:"group_name"`
	InvitedBy    uuid.UUID  `json:"invited_by"`
	RecipientID  uuid.UUID  `json:"recipient_id"`
	Email        string     `json:"email"`
	Role         string     `json:"role"`
	LocationName string     `json:"location_name,omitempty"`
	Status       string     `json:"status"`
	ExpiresAt    time.Time  `json:"expires_at"`
	RespondedAt  *time.Time `json:"responded_at,omitempty"`
	CreatedAt    time.Time  `json:"-"`
}

// Member represents either an active member or a pending member invitation.
type Member struct {
	MemberID     *uuid.UUID `json:"member_id,omitempty"`
	UserID       uuid.UUID  `json:"user_id"`
	FullName     string     `json:"full_name"`
	Username     string     `json:"username"`
	Email        string     `json:"email"`
	AvatarURL    string     `json:"avatar_url"`
	Role         string     `json:"role"`
	Status       string     `json:"status"`
	IsOwner      bool       `json:"is_owner"`
	AccessLevel  string     `json:"access_level"`
	LocationName *string    `json:"location_name,omitempty"`
	BalanceUSD   *int64     `json:"balance_usd,omitempty"`
	ProfitUZS    *int64     `json:"profit_uzs,omitempty"`
	InvitationID *uuid.UUID `json:"invitation_id,omitempty"`
	CreatedAt    time.Time  `json:"-"`
}

type ListMembersInput struct {
	Query  string
	Role   string
	Status string
}

type InviteInput struct {
	UserID       uuid.UUID
	Email        string
	Role         string
	LocationName string
}

type invitationNotificationPayload struct {
	EventType    string              `json:"event_type"`
	InvitationID uuid.UUID           `json:"invitation_id"`
	GroupID      uuid.UUID           `json:"group_id"`
	GroupName    string              `json:"group_name"`
	InvitedBy    invitationActor     `json:"invited_by"`
	Role         string              `json:"role"`
	Location     *invitationLocation `json:"location,omitempty"`
}

type invitationActor struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	AvatarURL string    `json:"avatar_url"`
}
type invitationLocation struct {
	Name string `json:"name"`
}

type notificationTranslations struct {
	English string
	Uzbek   string
	Russian string
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, now: time.Now}
}

func (s *Service) Create(ctx context.Context, actorID uuid.UUID, name string) (Group, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 255 {
		return Group{}, apperror.ErrInvalidData
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
		       CASE WHEN group_members.role::text = 'employee' THEN COALESCE((
		           SELECT SUM(profits.profit_uzs)::bigint
		           FROM member_profit_periods profits
		           WHERE profits.group_id = groups.id
		             AND profits.member_id = group_members.id
		             AND profits.deleted_at IS NULL
		       ), 0)::bigint END AS my_profit_uzs,
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
		return apperror.ErrInvalidData
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

func (s *Service) ListMembers(ctx context.Context, actorID, groupID uuid.UUID, input ListMembersInput) ([]Member, error) {
	if err := s.requireManager(ctx, actorID, groupID); err != nil {
		return nil, err
	}
	query, role, status, err := validMemberFilters(input)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		WITH balance_totals AS (
		    SELECT member_id, SUM(balance_usd)::bigint AS balance_usd
		    FROM employee_balances
		    WHERE group_id = $1 AND deleted_at IS NULL
		    GROUP BY member_id
		), profit_totals AS (
		    SELECT member_id, SUM(profit_uzs)::bigint AS profit_uzs
		    FROM member_profit_periods
		    WHERE group_id = $1 AND deleted_at IS NULL
		    GROUP BY member_id
		), member_rows AS (
		    SELECT members.id AS member_id, members.user_id,
		           COALESCE(
		               NULLIF(btrim(concat_ws(' ', users.first_name, users.last_name)), ''),
		               NULLIF(users.name, ''), NULLIF(users.username, ''), ''
		           ) AS full_name,
		           COALESCE(users.username, '') AS username,
		           COALESCE(users.email, '') AS email,
		           COALESCE(users.avatar_url, '') AS avatar_url,
		           members.role::text AS role, 'active'::text AS status,
		           members.is_owner,
		           CASE
		               WHEN members.is_owner THEN 'overall_control'
		               WHEN members.role::text = 'manager' THEN 'manage'
		               WHEN members.role::text = 'employee' THEN 'assigned'
		               ELSE 'read_only'
		           END AS access_level,
		           locations.name AS location_name,
		           CASE WHEN members.role::text = 'employee'
		                THEN COALESCE(balance_totals.balance_usd, 0)::bigint
		           END AS balance_usd,
		           CASE WHEN members.role::text = 'employee'
		                THEN COALESCE(profit_totals.profit_uzs, 0)::bigint
		           END AS profit_uzs,
		           NULL::uuid AS invitation_id, members.joined_at AS created_at
		    FROM group_members members
		    JOIN users ON users.id = members.user_id AND users.deleted_at IS NULL
		    LEFT JOIN locations
		      ON locations.group_id = members.group_id
		     AND locations.employee_id = members.id
		     AND locations.deleted_at IS NULL
		    LEFT JOIN balance_totals ON balance_totals.member_id = members.id
		    LEFT JOIN profit_totals ON profit_totals.member_id = members.id
		    WHERE members.group_id = $1 AND members.deleted_at IS NULL

		    UNION ALL

		    SELECT NULL::uuid AS member_id, invitations.invited_user_id AS user_id,
		           COALESCE(
		               NULLIF(btrim(concat_ws(' ', users.first_name, users.last_name)), ''),
		               NULLIF(users.name, ''), NULLIF(users.username, ''), ''
		           ) AS full_name,
		           COALESCE(users.username, '') AS username,
		           COALESCE(users.email, '') AS email,
		           COALESCE(users.avatar_url, '') AS avatar_url,
		           invitations.role::text AS role, 'pending'::text AS status,
		           false AS is_owner,
		           CASE
		               WHEN invitations.role::text = 'manager' THEN 'manage'
		               WHEN invitations.role::text = 'employee' THEN 'assigned'
		               ELSE 'read_only'
		           END AS access_level,
		           NULLIF(invitations.location_name, '') AS location_name,
		           NULL::bigint AS balance_usd, NULL::bigint AS profit_uzs,
		           invitations.id AS invitation_id, invitations.created_at
		    FROM group_invitations invitations
		    JOIN users ON users.id = invitations.invited_user_id AND users.deleted_at IS NULL
		    WHERE invitations.group_id = $1 AND invitations.status = 'pending'
		      AND invitations.deleted_at IS NULL
		      AND invitations.expires_at > now()
		)
		SELECT member_id, user_id, full_name, username, email, avatar_url,
		       role, status, is_owner, access_level, location_name,
		       balance_usd, profit_uzs, invitation_id, created_at
		FROM member_rows
		WHERE ($2::text = '' OR full_name ILIKE '%' || $2 || '%'
		       OR username ILIKE '%' || $2 || '%'
		       OR email ILIKE '%' || $2 || '%'
		       OR COALESCE(location_name, '') ILIKE '%' || $2 || '%')
		  AND ($3::text = '' OR role = $3)
		  AND ($4::text = '' OR status = $4)
		ORDER BY is_owner DESC,
		         CASE role WHEN 'manager' THEN 1 WHEN 'employee' THEN 2 WHEN 'investor' THEN 3 ELSE 4 END,
		         created_at ASC, user_id ASC
	`, groupID, helpers.EscapeLike(query), role, status)
	if err != nil {
		return nil, fmt.Errorf("listing group members: %w", err)
	}
	defer rows.Close()
	members := make([]Member, 0)
	for rows.Next() {
		var member Member
		if err := rows.Scan(
			&member.MemberID, &member.UserID, &member.FullName, &member.Username,
			&member.Email, &member.AvatarURL, &member.Role, &member.Status,
			&member.IsOwner, &member.AccessLevel, &member.LocationName,
			&member.BalanceUSD, &member.ProfitUZS, &member.InvitationID, &member.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning group member: %w", err)
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating group members: %w", err)
	}
	return members, nil
}

func validMemberFilters(input ListMembersInput) (string, string, string, error) {
	query := strings.TrimSpace(input.Query)
	if len(query) > 100 {
		return "", "", "", apperror.ErrInvalidData
	}
	role := strings.ToLower(strings.TrimSpace(input.Role))
	if role == "all" {
		role = ""
	}
	switch role {
	case "", "manager", "employee", "investor":
	default:
		return "", "", "", apperror.ErrInvalidData
	}
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status == "all" {
		status = ""
	}
	switch status {
	case "", "active", "pending":
	default:
		return "", "", "", apperror.ErrInvalidData
	}
	return query, role, status, nil
}

func (s *Service) Invite(ctx context.Context, actorID, groupID uuid.UUID, input InviteInput) (Invitation, error) {
	role, err := invitationRole(input.Role)
	if err != nil {
		return Invitation{}, err
	}
	if err := s.requireManager(ctx, actorID, groupID); err != nil {
		return Invitation{}, err
	}
	input.LocationName = strings.TrimSpace(input.LocationName)
	if len(input.LocationName) > 255 {
		return Invitation{}, apperror.ErrInvalidData
	}

	recipientID, email, err := s.findRecipient(ctx, input)
	if err != nil {
		return Invitation{}, err
	}

	invitation := Invitation{
		ID:           uuid.New(),
		GroupID:      groupID,
		InvitedBy:    actorID,
		RecipientID:  recipientID,
		Email:        email,
		Role:         role,
		LocationName: input.LocationName,
		Status:       "pending",
		CreatedAt:    s.now().UTC(),
	}
	invitation.ExpiresAt = invitation.CreatedAt.Add(invitationLifetime)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Invitation{}, fmt.Errorf("beginning invitation creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Expired invitations keep status 'pending' until something closes them, and
	// the one-pending-invitation index would otherwise block re-inviting forever.
	_, err = tx.Exec(ctx, `
		UPDATE group_invitations
		SET status = 'revoked', revoked_at = $3, updated_at = $3
		WHERE group_id = $1 AND invited_user_id = $2
		  AND status = 'pending' AND expires_at <= $3 AND deleted_at IS NULL
	`, groupID, recipientID, invitation.CreatedAt)
	if err != nil {
		return Invitation{}, fmt.Errorf("revoking expired invitations: %w", err)
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO group_invitations (
			id, group_id, invited_by, invited_user_id, email, role, location_name, status, expires_at, created_at
		)
		SELECT $1, $2, $3, $4, $5, $6::user_role, $7, $8, $9, $10
		WHERE NOT EXISTS (
			SELECT 1 FROM group_members
			WHERE group_id = $2 AND user_id = $4 AND deleted_at IS NULL
		)
		RETURNING id
	`, invitation.ID, invitation.GroupID, invitation.InvitedBy, invitation.RecipientID,
		invitation.Email, invitation.Role, invitation.LocationName, invitation.Status, invitation.ExpiresAt, invitation.CreatedAt).Scan(&invitation.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Invitation{}, apperror.ErrAlreadyExists
	}
	if err != nil {
		return Invitation{}, fmt.Errorf("creating invitation: %w", err)
	}
	var invitedBy invitationActor
	err = tx.QueryRow(ctx, `
		SELECT groups.name, users.id, COALESCE(users.name, ''), COALESCE(users.avatar_url, '')
		FROM groups, users
		WHERE groups.id = $1 AND users.id = $2
	`, groupID, actorID).Scan(&invitation.GroupName, &invitedBy.ID, &invitedBy.Name, &invitedBy.AvatarURL)
	if err != nil {
		return Invitation{}, fmt.Errorf("getting invitation group and sender: %w", err)
	}
	payloadData := invitationNotificationPayload{
		EventType:    "GROUP_INVITATION",
		InvitationID: invitation.ID,
		GroupID:      invitation.GroupID,
		GroupName:    invitation.GroupName,
		InvitedBy:    invitedBy,
		Role:         invitation.Role,
	}
	if invitation.LocationName != "" {
		payloadData.Location = &invitationLocation{Name: invitation.LocationName}
	}
	payload, err := json.Marshal(payloadData)
	if err != nil {
		return Invitation{}, fmt.Errorf("encoding invitation notification: %w", err)
	}
	titleTranslations, contentTranslations := groupInvitationNotificationTranslations(invitation.GroupName)
	_, err = tx.Exec(ctx, `
		WITH notification AS (
			INSERT INTO notifications (
				title_en, title_uz, title_ru, content_en, content_uz, content_ru,
				type, payload, created_at, expires_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, 'TARGETED'::notification_type, $7, $8, $9)
			RETURNING id
		)
		INSERT INTO notification_recipients (notification_id, user_id, is_read, created_at, updated_at)
		SELECT id, $10, false, $8, $8 FROM notification
	`, titleTranslations.English, titleTranslations.Uzbek, titleTranslations.Russian,
		contentTranslations.English, contentTranslations.Uzbek, contentTranslations.Russian,
		payload, invitation.CreatedAt, invitation.ExpiresAt, invitation.RecipientID)
	if err != nil {
		return Invitation{}, fmt.Errorf("creating invitation notification: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Invitation{}, fmt.Errorf("committing invitation creation: %w", err)
	}
	return invitation, nil
}

func groupInvitationNotificationTranslations(groupName string) (notificationTranslations, notificationTranslations) {
	return notificationTranslations{
		English: "Group invitation",
		Uzbek:   "Guruhga taklif",
		Russian: "Приглашение в группу",
	}, notificationTranslations{
		English: "You have been invited to join " + groupName,
		Uzbek:   "Siz " + groupName + " guruhiga qo'shilish uchun taklif qilindingiz",
		Russian: "Вас пригласили присоединиться к группе " + groupName,
	}
}

func (s *Service) RespondInvitation(ctx context.Context, actorID, invitationID uuid.UUID, action string) (Invitation, error) {
	if action != "accept" && action != "reject" {
		return Invitation{}, apperror.ErrInvalidData
	}
	status := "accepted"
	if action == "reject" {
		status = "rejected"
	}
	return s.respond(ctx, actorID, invitationID, status)
}

func (s *Service) respond(ctx context.Context, actorID, invitationID uuid.UUID, status string) (Invitation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Invitation{}, fmt.Errorf("beginning invitation response: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var invitation Invitation
	err = tx.QueryRow(ctx, `
		SELECT invitations.id, invitations.group_id, groups.name, invitations.invited_by,
		       invitations.invited_user_id, invitations.email, invitations.role::text,
		       invitations.status, invitations.expires_at, invitations.responded_at, invitations.created_at
		FROM group_invitations invitations
		JOIN groups ON groups.id = invitations.group_id
		WHERE invitations.id = $1
		  AND invitations.invited_user_id IS NOT NULL
		  AND invitations.deleted_at IS NULL
		FOR UPDATE OF invitations
	`, invitationID).Scan(
		&invitation.ID, &invitation.GroupID, &invitation.GroupName, &invitation.InvitedBy,
		&invitation.RecipientID, &invitation.Email, &invitation.Role, &invitation.Status,
		&invitation.ExpiresAt, &invitation.RespondedAt, &invitation.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Invitation{}, apperror.ErrRecordNotFound
	}
	if err != nil {
		return Invitation{}, fmt.Errorf("locking invitation: %w", err)
	}
	if invitation.RecipientID != actorID {
		return Invitation{}, apperror.ErrForbidden
	}
	if invitation.Status != "pending" || !invitation.ExpiresAt.After(s.now().UTC()) {
		return Invitation{}, apperror.ErrConflict
	}

	now := s.now().UTC()
	if status == "accepted" {
		var username string
		err = tx.QueryRow(ctx, `SELECT username FROM users WHERE id = $1 AND deleted_at IS NULL`, actorID).Scan(&username)
		if errors.Is(err, pgx.ErrNoRows) {
			return Invitation{}, apperror.ErrRecordNotFound
		}
		if err != nil {
			return Invitation{}, fmt.Errorf("getting invitation recipient: %w", err)
		}
		var membershipID uuid.UUID
		err = tx.QueryRow(ctx, `
			INSERT INTO group_members (id, group_id, user_id, username, role, is_owner, joined_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5::user_role, false, $6, $6, $6)
			RETURNING id
		`, uuid.New(), invitation.GroupID, actorID, username, invitation.Role, now).Scan(&membershipID)
		if err != nil {
			return Invitation{}, fmt.Errorf("adding invited member: %w", err)
		}
		if invitation.Role == "employee" {
			_, err = tx.Exec(ctx, `
				INSERT INTO employee_balances (group_id, member_id, balance_usd, created_at, updated_at)
				VALUES ($1, $2, 0, $3, $3)
			`, invitation.GroupID, membershipID, now)
			if err != nil {
				return Invitation{}, fmt.Errorf("creating employee balance: %w", err)
			}
		}
	}

	_, err = tx.Exec(ctx, `
		UPDATE group_invitations
		SET status = $2::varchar, responded_at = $3,
		    accepted_at = CASE WHEN $2::text = 'accepted' THEN $3 ELSE accepted_at END,
		    rejected_at = CASE WHEN $2::text = 'rejected' THEN $3 ELSE rejected_at END,
		    updated_at = $3
		WHERE id = $1
	`, invitationID, status, now)
	if err != nil {
		return Invitation{}, fmt.Errorf("updating invitation status: %w", err)
	}
	_, err = tx.Exec(ctx, `
		UPDATE notification_recipients recipients
		SET is_read = true, read_at = $3, updated_at = $3
		FROM notifications
		WHERE recipients.notification_id = notifications.id
		  AND recipients.user_id = $1
		  AND notifications.payload->>'event_type' = 'GROUP_INVITATION'
		  AND notifications.payload->>'invitation_id' = $2
		  AND recipients.is_read = false
		  AND recipients.deleted_at IS NULL
		  AND notifications.deleted_at IS NULL
	`, actorID, invitationID.String(), now)
	if err != nil {
		return Invitation{}, fmt.Errorf("marking invitation notification read: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Invitation{}, fmt.Errorf("committing invitation response: %w", err)
	}
	invitation.Status = status
	invitation.RespondedAt = &now
	return invitation, nil
}

func (s *Service) requireManager(ctx context.Context, actorID, groupID uuid.UUID) error {
	var allowed bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM group_members
			JOIN groups ON groups.id = group_members.group_id
			WHERE group_members.group_id = $1
			  AND group_members.user_id = $2
			  AND group_members.deleted_at IS NULL
			  AND groups.deleted_at IS NULL
			  AND groups.is_active
			  AND (group_members.is_owner OR group_members.role::text IN ('owner', 'admin', 'manager'))
		)
	`, groupID, actorID).Scan(&allowed)
	if err != nil {
		return fmt.Errorf("checking group manager permission: %w", err)
	}
	if !allowed {
		return apperror.ErrForbidden
	}
	return nil
}

func (s *Service) findRecipient(ctx context.Context, input InviteInput) (uuid.UUID, string, error) {
	if input.UserID == uuid.Nil && strings.TrimSpace(input.Email) == "" {
		return uuid.Nil, "", apperror.ErrInvalidData
	}
	if input.UserID != uuid.Nil && strings.TrimSpace(input.Email) != "" {
		return uuid.Nil, "", apperror.ErrInvalidData
	}

	var recipientID uuid.UUID
	var email string
	var err error
	if input.UserID != uuid.Nil {
		err = s.pool.QueryRow(ctx, `SELECT id, email FROM users WHERE id = $1 AND deleted_at IS NULL AND is_active`, input.UserID).Scan(&recipientID, &email)
	} else {
		email, err = helpers.NormalizeEmail(input.Email)
		if err != nil {
			return uuid.Nil, "", apperror.ErrInvalidData
		}
		// Stored emails are normalized to lowercase, so plain equality can use users_email_unique_idx.
		err = s.pool.QueryRow(ctx, `SELECT id, email FROM users WHERE email = $1 AND deleted_at IS NULL AND is_active`, email).Scan(&recipientID, &email)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", apperror.ErrRecordNotFound
	}
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("finding invitation recipient: %w", err)
	}
	return recipientID, email, nil
}

func invitationRole(value string) (string, error) {
	role := strings.ToLower(strings.TrimSpace(value))
	switch role {
	case "employee", "manager", "investor":
		return role, nil
	default:
		return "", apperror.ErrInvalidData
	}
}
