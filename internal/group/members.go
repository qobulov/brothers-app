package group

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/helpers"
)

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
