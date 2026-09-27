package group

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qobulov/brothers-app/pkg/database"
)

func TestService_CreateInviteAndAccept(t *testing.T) {
	pool, cleanup := database.SetupTestDB(t)
	defer cleanup()

	ownerID := createUser(t, pool, "owner@example.com", "group-owner")
	recipientID := createUser(t, pool, "employee@example.com", "group-employee")
	service := NewService(pool)

	created, err := service.Create(context.Background(), ownerID, "Tashkent ↔ Kokand")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if !created.IsOwner || created.Role != "manager" {
		t.Fatalf("owner group = %#v, want manager owner", created)
	}

	invitation, err := service.Invite(context.Background(), ownerID, created.ID, InviteInput{
		Email: "employee@example.com",
		Role:  "employee",
	})
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}
	if invitation.RecipientID != recipientID || invitation.Status != "pending" {
		t.Fatalf("invitation = %#v, want pending invitation for recipient", invitation)
	}
	var notification struct {
		ID        int64
		Type      string          `json:"type"`
		ActionURL string          `json:"action_url"`
		Payload   json.RawMessage `json:"payload"`
		Recipient uuid.UUID       `json:"recipient"`
		IsRead    bool            `json:"is_read"`
	}
	err = pool.QueryRow(context.Background(), `
		SELECT notifications.id, notifications.type::text, notifications.action_url, notifications.payload,
		       notification_recipients.user_id, notification_recipients.is_read
		FROM notifications
		JOIN notification_recipients ON notification_recipients.notification_id = notifications.id
		WHERE notification_recipients.user_id = $1
	`, recipientID).Scan(
		&notification.ID, &notification.Type, &notification.ActionURL, &notification.Payload,
		&notification.Recipient, &notification.IsRead,
	)
	if err != nil {
		t.Fatalf("get invitation notification: %v", err)
	}
	if notification.Type != "TARGETED" || notification.Recipient != recipientID || notification.IsRead {
		t.Fatalf("notification = %#v, want unread targeted notification for recipient", notification)
	}
	var payload struct {
		EventType    string    `json:"event_type"`
		InvitationID uuid.UUID `json:"invitation_id"`
	}
	if err := json.Unmarshal(notification.Payload, &payload); err != nil {
		t.Fatalf("decode notification payload: %v", err)
	}
	if payload.EventType != "GROUP_INVITATION" || payload.InvitationID != invitation.ID {
		t.Fatalf("notification payload = %#v, want invitation %s", payload, invitation.ID)
	}

	pending, err := service.ListMembers(context.Background(), ownerID, created.ID)
	if err != nil {
		t.Fatalf("list pending members: %v", err)
	}
	if len(pending) != 2 || pending[1].Status != "pending" || pending[1].InvitationID == nil || *pending[1].InvitationID != invitation.ID {
		t.Fatalf("members = %#v, want pending member invitation %s", pending, invitation.ID)
	}

	accepted, err := service.RespondInvitation(context.Background(), recipientID, invitation.ID, "accept")
	if err != nil {
		t.Fatalf("accept invitation: %v", err)
	}
	if accepted.Status != "accepted" || accepted.RespondedAt == nil {
		t.Fatalf("accepted invitation = %#v, want accepted with response time", accepted)
	}
	var notificationRead bool
	err = pool.QueryRow(context.Background(), `
		SELECT notification_recipients.is_read
		FROM notification_recipients
		JOIN notifications ON notifications.id = notification_recipients.notification_id
		WHERE notification_recipients.user_id = $1
		  AND notifications.payload->>'invitation_id' = $2
	`, recipientID, invitation.ID.String()).Scan(&notificationRead)
	if err != nil {
		t.Fatalf("get invitation notification read state: %v", err)
	}
	if !notificationRead {
		t.Fatal("invitation notification must be marked read after acceptance")
	}

	groups, err := service.List(context.Background(), recipientID)
	if err != nil {
		t.Fatalf("list recipient groups: %v", err)
	}
	if len(groups) != 1 || groups[0].ID != created.ID || groups[0].Role != "employee" {
		t.Fatalf("recipient groups = %#v, want employee membership", groups)
	}
}

func createUser(t *testing.T, pool *pgxpool.Pool, email, username string) uuid.UUID {
	t.Helper()
	userID := uuid.New()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO users (id, email, username, language, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, 'uz', true, now(), now())
	`, userID, email, username)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	return userID
}
