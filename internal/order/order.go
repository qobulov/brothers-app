// Package order implements two-employee money transfer orders inside a group.
// The giver takes cash from one customer, the receiver pays another; each
// confirms the amount independently and the order completes when they agree.
package order

import (
	"time"

	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/apperror"
)

const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusCancelled = "cancelled"

	StateWaitingForYou          = "waiting_for_you"
	StateWaitingForConfirmation = "waiting_for_confirmation"
	StateAmountMismatch         = "amount_mismatch"
	StateCancellationRequested  = "cancellation_requested"

	RoleGiver    = "giver"
	RoleReceiver = "receiver"

	maxAmountUSD int64 = 1_000_000_000
	maxFeeUZS    int64 = 1_000_000_000_000

	defaultPageSize = 50
	maxPageSize     = 100
	maxOffset       = 10000
)

type Location struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type Party struct {
	UserID        uuid.UUID `json:"user_id"`
	Name          string    `json:"name"`
	AvatarURL     string    `json:"avatar_url"`
	Location      *Location `json:"location"`
	CustomerPhone string    `json:"customer_phone"`
}

type PartySummary struct {
	UserID       uuid.UUID `json:"user_id"`
	Name         string    `json:"name"`
	LocationName string    `json:"location_name,omitempty"`
}

type Actor struct {
	UserID uuid.UUID `json:"user_id"`
	Name   string    `json:"name"`
}

// Confirmation fields the viewer may not see are nil and omitted from JSON.
type Confirmation struct {
	Role        string     `json:"role" enums:"giver,receiver"`
	UserID      uuid.UUID  `json:"user_id"`
	Status      string     `json:"status" enums:"pending,confirmed"`
	AmountUSD   *int64     `json:"amount_usd,omitempty"`
	FeeUZS      *int64     `json:"fee_uzs,omitempty"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
}

type Order struct {
	ID            uuid.UUID      `json:"id"`
	GroupID       uuid.UUID      `json:"group_id"`
	Status        string         `json:"status" enums:"pending,completed,cancelled"`
	State         string         `json:"state" enums:"waiting_for_you,waiting_for_confirmation,amount_mismatch,cancellation_requested,completed,cancelled"`
	AmountUSD     int64          `json:"amount_usd"`
	FeeUZS        int64          `json:"fee_uzs"`
	Giver         Party          `json:"giver"`
	Receiver      Party          `json:"receiver"`
	CreatedBy     Actor          `json:"created_by"`
	Confirmations []Confirmation `json:"confirmations"`
	Cancellation  *Cancellation  `json:"cancellation"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	CompletedAt   *time.Time     `json:"completed_at"`
}

type ListItem struct {
	ID        uuid.UUID    `json:"id"`
	Status    string       `json:"status" enums:"pending,completed,cancelled"`
	State     string       `json:"state" enums:"waiting_for_you,waiting_for_confirmation,amount_mismatch,cancellation_requested,completed,cancelled"`
	AmountUSD int64        `json:"amount_usd"`
	Giver     PartySummary `json:"giver"`
	Receiver  PartySummary `json:"receiver"`
	CreatedAt time.Time    `json:"created_at"`
}

// Cancellation is the order's open request, or the approved one once the
// order is cancelled. Rejected requests are not shown.
type Cancellation struct {
	ID          uuid.UUID  `json:"id"`
	Status      string     `json:"status" enums:"pending,approved"`
	RequestedBy Actor      `json:"requested_by"`
	Reason      string     `json:"reason,omitempty"`
	RequestedAt time.Time  `json:"requested_at"`
	RespondedAt *time.Time `json:"responded_at,omitempty"`
	Approvals   []Approval `json:"approvals"`
}

type Approval struct {
	Role   string    `json:"role" enums:"giver,receiver"`
	UserID uuid.UUID `json:"user_id"`
	Status string    `json:"status" enums:"approved,waiting"`
}

type Event struct {
	ID        uuid.UUID      `json:"id"`
	EventType string         `json:"event_type" enums:"created,updated,confirmed,confirmation_corrected,amount_mismatch,completed,cancellation_requested,cancellation_rejected,cancelled"`
	Actor     Actor          `json:"actor"`
	Payload   map[string]any `json:"payload" swaggertype:"object"`
	CreatedAt time.Time      `json:"created_at"`
}

type CreateInput struct {
	GiverUserID           uuid.UUID
	GiverCustomerPhone    string
	ReceiverUserID        uuid.UUID
	ReceiverCustomerPhone string
	AmountUSD             int64
	FeeUZS                int64
}

// EditInput changes only the non-nil fields.
type EditInput struct {
	GiverUserID           *uuid.UUID
	GiverCustomerPhone    *string
	ReceiverUserID        *uuid.UUID
	ReceiverCustomerPhone *string
	AmountUSD             *int64
	FeeUZS                *int64
}

type ConfirmInput struct {
	AmountUSD int64
	FeeUZS    int64
}

// ListInput with Limit 0 uses the default page size.
type ListInput struct {
	Status string
	Limit  int
	Offset int
}

// viewer is the requesting user's active membership in the order's group.
type viewer struct {
	userID   uuid.UUID
	memberID uuid.UUID
	role     string
	isOwner  bool
}

func (v viewer) isManager() bool {
	return v.isOwner || v.role == "manager" || v.role == "owner" || v.role == "admin"
}

func (v viewer) isEmployee() bool { return v.role == "employee" }

func (v viewer) isParty(p parties) bool {
	return v.memberID == p.giverMemberID || v.memberID == p.receiverMemberID
}

// canSee hides other people's orders from employees; everyone else sees all.
func (v viewer) canSee(p parties) bool { return !v.isEmployee() || v.isParty(p) }

type parties struct {
	giverMemberID, receiverMemberID uuid.UUID
	giverUserID, receiverUserID     uuid.UUID
}

type confirmationRow struct {
	memberID    uuid.UUID
	amountUSD   int64
	feeUZS      int64
	confirmedAt time.Time
}

// present computes the viewer's state and hides numbers the viewer may not
// see: a party sees the counterparty's amount only after confirming, and
// never the counterparty's fee.
func present(status string, p parties, rows []confirmationRow, v viewer) (string, []Confirmation) {
	pr := newPresentation(p, rows, v)
	confirmations := []Confirmation{
		pr.confirmation(RoleGiver, p.giverMemberID, p.giverUserID),
		pr.confirmation(RoleReceiver, p.receiverMemberID, p.receiverUserID),
	}
	return pr.state(status, p), confirmations
}

type presentation struct {
	viewer          viewer
	byMember        map[uuid.UUID]confirmationRow
	viewerIsParty   bool
	viewerConfirmed bool
}

func newPresentation(p parties, rows []confirmationRow, v viewer) presentation {
	byMember := make(map[uuid.UUID]confirmationRow, len(rows))
	for _, row := range rows {
		byMember[row.memberID] = row
	}
	_, confirmed := byMember[v.memberID]
	return presentation{viewer: v, byMember: byMember, viewerIsParty: v.isParty(p), viewerConfirmed: confirmed}
}

func (pr presentation) confirmation(role string, memberID, userID uuid.UUID) Confirmation {
	result := Confirmation{Role: role, UserID: userID, Status: "pending"}
	row, ok := pr.byMember[memberID]
	if !ok {
		return result
	}
	confirmedAt := row.confirmedAt
	result.Status = "confirmed"
	result.ConfirmedAt = &confirmedAt
	own := memberID == pr.viewer.memberID
	if !pr.viewerIsParty || own || pr.viewerConfirmed {
		amount := row.amountUSD
		result.AmountUSD = &amount
	}
	if !pr.viewerIsParty || own {
		fee := row.feeUZS
		result.FeeUZS = &fee
	}
	return result
}

func (pr presentation) state(status string, p parties) string {
	if status != StatusPending {
		return status
	}
	giver, giverConfirmed := pr.byMember[p.giverMemberID]
	receiver, receiverConfirmed := pr.byMember[p.receiverMemberID]
	switch {
	case giverConfirmed && receiverConfirmed && giver.amountUSD != receiver.amountUSD:
		return StateAmountMismatch
	case pr.viewerIsParty && !pr.viewerConfirmed:
		return StateWaitingForYou
	default:
		return StateWaitingForConfirmation
	}
}

// notPending explains why a completed or cancelled order cannot change.
func notPending(status string) error {
	if status == StatusCancelled {
		return apperror.New(apperror.ErrConflict, apperror.Text{
			UZ: "Buyurtma bekor qilingan, uni o'zgartirib bo'lmaydi",
			RU: "Заказ отменён, его нельзя изменить",
			EN: "The order is cancelled and cannot change",
		})
	}
	return apperror.New(apperror.ErrConflict, apperror.Text{
		UZ: "Buyurtma yakunlangan, uni o'zgartirib bo'lmaydi",
		RU: "Заказ выполнен, его нельзя изменить",
		EN: "The order is completed and cannot change",
	})
}
