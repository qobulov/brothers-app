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
	UserID       uuid.UUID  `json:"user_id"`
	Username     string     `json:"username"`
	Email        string     `json:"email"`
	AvatarURL    string     `json:"avatar_url"`
	Role         string     `json:"role"`
	Status       string     `json:"status"`
	IsOwner      bool       `json:"is_owner"`
	InvitationID *uuid.UUID `json:"invitation_id,omitempty"`
	CreatedAt    time.Time  `json:"-"`
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
	_, err = tx.Exec(ctx, `
		INSERT INTO group_members (id, group_id, user_id, username, role, is_owner, joined_at, created_at, updated_at)
		SELECT $1, $2, id, username, 'manager'::user_role, true, $3, $3, $3
		FROM users
		WHERE id = $4 AND deleted_at IS NULL
	`, uuid.New(), groupID, now, actorID)
	if err != nil {
		return Group{}, fmt.Errorf("creating owner membership: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Group{}, fmt.Errorf("committing group creation: %w", err)
	}
	return Group{ID: groupID, Name: name, Role: "manager", IsOwner: true, CreatedAt: now}, nil
}

func (s *Service) List(ctx context.Context, actorID uuid.UUID) ([]Group, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT groups.id, groups.name, group_members.role::text, group_members.is_owner, groups.created_at
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

	groups := make([]Group, 0)
	for rows.Next() {
		var item Group
		if err := rows.Scan(&item.ID, &item.Name, &item.Role, &item.IsOwner, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning group: %w", err)
		}
		groups = append(groups, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating groups: %w", err)
	}
	return groups, nil
}

func (s *Service) Get(ctx context.Context, actorID, groupID uuid.UUID) (Group, error) {
	var item Group
	err := s.pool.QueryRow(ctx, `
		SELECT groups.id, groups.name, group_members.role::text, group_members.is_owner, groups.created_at
		FROM group_members
		JOIN groups ON groups.id = group_members.group_id
		WHERE group_members.user_id = $1
		  AND groups.id = $2
		  AND group_members.deleted_at IS NULL
		  AND groups.deleted_at IS NULL
		  AND groups.is_active
	`, actorID, groupID).Scan(&item.ID, &item.Name, &item.Role, &item.IsOwner, &item.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Group{}, apperror.ErrRecordNotFound
	}
	if err != nil {
		return Group{}, fmt.Errorf("getting group: %w", err)
	}
	return item, nil
}

func (s *Service) ListMembers(ctx context.Context, actorID, groupID uuid.UUID) ([]Member, error) {
	if err := s.requireManager(ctx, actorID, groupID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT members.user_id, COALESCE(users.username, ''), COALESCE(users.email, ''), COALESCE(users.avatar_url, ''),
		       members.role::text, 'active', members.is_owner, NULL::uuid, members.joined_at
		FROM group_members members
		JOIN users ON users.id = members.user_id
		WHERE members.group_id = $1 AND members.deleted_at IS NULL
		UNION ALL
		SELECT invitations.invited_user_id, COALESCE(users.username, ''), COALESCE(users.email, ''), COALESCE(users.avatar_url, ''),
		       invitations.role::text, 'pending', false, invitations.id, invitations.created_at
		FROM group_invitations invitations
		JOIN users ON users.id = invitations.invited_user_id
		WHERE invitations.group_id = $1 AND invitations.status = 'pending'
		  AND invitations.expires_at > now()
		ORDER BY 9 ASC
	`, groupID)
	if err != nil {
		return nil, fmt.Errorf("listing group members: %w", err)
	}
	defer rows.Close()
	members := make([]Member, 0)
	for rows.Next() {
		var member Member
		if err := rows.Scan(&member.UserID, &member.Username, &member.Email, &member.AvatarURL, &member.Role, &member.Status, &member.IsOwner, &member.InvitationID, &member.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning group member: %w", err)
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating group members: %w", err)
	}
	return members, nil
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
	var alreadyMember bool
	err = s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM group_members
			WHERE group_id = $1 AND user_id = $2 AND deleted_at IS NULL
		)
	`, groupID, recipientID).Scan(&alreadyMember)
	if err != nil {
		return Invitation{}, fmt.Errorf("checking group membership: %w", err)
	}
	if alreadyMember {
		return Invitation{}, apperror.ErrAlreadyExists
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

	err = tx.QueryRow(ctx, `
		INSERT INTO group_invitations (
			id, group_id, invited_by, invited_user_id, email, role, location_name, status, expires_at, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6::user_role, $7, $8, $9, $10)
		RETURNING id
	`, invitation.ID, invitation.GroupID, invitation.InvitedBy, invitation.RecipientID,
		invitation.Email, invitation.Role, invitation.LocationName, invitation.Status, invitation.ExpiresAt, invitation.CreatedAt).Scan(&invitation.ID)
	if err != nil {
		return Invitation{}, fmt.Errorf("creating invitation: %w", err)
	}
	if err := tx.QueryRow(ctx, `SELECT name FROM groups WHERE id = $1`, groupID).Scan(&invitation.GroupName); err != nil {
		return Invitation{}, fmt.Errorf("getting invitation group: %w", err)
	}
	var invitedBy invitationActor
	if err := tx.QueryRow(ctx, `SELECT id, COALESCE(name, ''), COALESCE(avatar_url, '') FROM users WHERE id = $1`, actorID).Scan(&invitedBy.ID, &invitedBy.Name, &invitedBy.AvatarURL); err != nil {
		return Invitation{}, fmt.Errorf("getting invitation sender: %w", err)
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
	var notificationID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO notifications (title, content, type, action_url, payload, created_at, expires_at)
		VALUES ($1, $2, 'TARGETED'::notification_type, $3, $4, $5, $6)
		RETURNING id
	`, "Group invitation", "You have been invited to join "+invitation.GroupName,
		"/invitations/"+invitation.ID.String()+"/action", payload, invitation.CreatedAt, invitation.ExpiresAt).Scan(&notificationID)
	if err != nil {
		return Invitation{}, fmt.Errorf("creating invitation notification: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO notification_recipients (notification_id, user_id, is_read)
		VALUES ($1, $2, false)
	`, notificationID, invitation.RecipientID)
	if err != nil {
		return Invitation{}, fmt.Errorf("creating invitation notification recipient: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Invitation{}, fmt.Errorf("committing invitation creation: %w", err)
	}
	return invitation, nil
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
		_, err = tx.Exec(ctx, `
			INSERT INTO group_members (id, group_id, user_id, username, role, is_owner, joined_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5::user_role, false, $6, $6, $6)
		`, uuid.New(), invitation.GroupID, actorID, username, invitation.Role, now)
		if err != nil {
			return Invitation{}, fmt.Errorf("adding invited member: %w", err)
		}
	}

	_, err = tx.Exec(ctx, `
		UPDATE group_invitations
		SET status = $2::varchar, responded_at = $3,
		    accepted_at = CASE WHEN $2::text = 'accepted' THEN $3 ELSE accepted_at END,
		    rejected_at = CASE WHEN $2::text = 'rejected' THEN $3 ELSE rejected_at END
		WHERE id = $1
	`, invitationID, status, now)
	if err != nil {
		return Invitation{}, fmt.Errorf("updating invitation status: %w", err)
	}
	_, err = tx.Exec(ctx, `
		UPDATE notification_recipients recipients
		SET is_read = true, read_at = $3
		FROM notifications
		WHERE recipients.notification_id = notifications.id
		  AND recipients.user_id = $1
		  AND notifications.payload->>'event_type' = 'GROUP_INVITATION'
		  AND notifications.payload->>'invitation_id' = $2
		  AND recipients.is_read = false
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
		err = s.pool.QueryRow(ctx, `SELECT id, email FROM users WHERE lower(email) = lower($1) AND deleted_at IS NULL AND is_active`, email).Scan(&recipientID, &email)
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
