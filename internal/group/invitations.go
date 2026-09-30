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
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/helpers"
)

const invitationLifetime = 7 * 24 * time.Hour

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
		return Invitation{}, apperror.New(apperror.ErrInvalidData, apperror.Text{UZ: "Joy nomi 255 belgidan oshmasligi kerak", RU: "Название локации не должно превышать 255 символов", EN: "The location name must be at most 255 characters"})
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
		return Invitation{}, apperror.New(apperror.ErrInvalidData, apperror.Text{UZ: "Amal accept yoki reject bo'lishi kerak", RU: "Действие должно быть accept или reject", EN: "Action must be accept or reject"})
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

func (s *Service) findRecipient(ctx context.Context, input InviteInput) (uuid.UUID, string, error) {
	if input.UserID == uuid.Nil && strings.TrimSpace(input.Email) == "" {
		return uuid.Nil, "", apperror.New(apperror.ErrInvalidData, apperror.Text{UZ: "Taklif uchun user_id yoki email kiriting", RU: "Укажите user_id или email для приглашения", EN: "Provide a user_id or an email to invite"})
	}
	if input.UserID != uuid.Nil && strings.TrimSpace(input.Email) != "" {
		return uuid.Nil, "", apperror.New(apperror.ErrInvalidData, apperror.Text{UZ: "user_id yoki email'dan faqat bittasini kiriting", RU: "Укажите только user_id или только email", EN: "Provide either a user_id or an email, not both"})
	}

	var recipientID uuid.UUID
	var email string
	var err error
	if input.UserID != uuid.Nil {
		err = s.pool.QueryRow(ctx, `SELECT id, email FROM users WHERE id = $1 AND deleted_at IS NULL AND is_active`, input.UserID).Scan(&recipientID, &email)
	} else {
		email, err = helpers.NormalizeEmail(input.Email)
		if err != nil {
			return uuid.Nil, "", apperror.InvalidEmail()
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
		return "", apperror.New(apperror.ErrInvalidData, apperror.Text{UZ: "Rol employee, manager yoki investor bo'lishi kerak", RU: "Роль должна быть employee, manager или investor", EN: "Role must be employee, manager or investor"})
	}
}
