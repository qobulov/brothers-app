package group

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/database"
	"github.com/qobulov/brothers-app/pkg/paging"
)

const maxBalanceUSD int64 = 1_000_000_000_000

const (
	maxAdjustmentReason = 500
)

type PersonRef struct {
	UserID uuid.UUID `json:"user_id"`
	Name   string    `json:"name"`
}

type BalanceAdjustment struct {
	ID            uuid.UUID `json:"id"`
	Direction     string    `json:"direction" enums:"increase,decrease"`
	AmountUSD     int64     `json:"amount_usd"`
	OldBalanceUSD int64     `json:"old_balance_usd"`
	NewBalanceUSD int64     `json:"new_balance_usd"`
	Reason        string    `json:"reason,omitempty"`
	ChangedBy     PersonRef `json:"changed_by"`
	CreatedAt     time.Time `json:"created_at"`
}

type BalanceMember struct {
	UserID       uuid.UUID `json:"user_id"`
	FullName     string    `json:"full_name"`
	AvatarURL    string    `json:"avatar_url"`
	Role         string    `json:"role"`
	LocationName string    `json:"location_name,omitempty"`
}

// BalanceHistory lists manual adjustments only; order effects are in order history.
type BalanceHistory struct {
	Member            BalanceMember       `json:"member"`
	CurrentBalanceUSD int64               `json:"current_balance_usd"`
	Adjustments       []BalanceAdjustment `json:"adjustments"`
}

type AdjustBalanceInput struct {
	NewBalanceUSD int64
	Reason        string
}

// AdjustBalance sets an employee's balance and records the change permanently.
func (s *Service) AdjustBalance(ctx context.Context, actorID, groupID, userID uuid.UUID, input AdjustBalanceInput) (BalanceAdjustment, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	if input.NewBalanceUSD < -maxBalanceUSD || input.NewBalanceUSD > maxBalanceUSD {
		return BalanceAdjustment{}, apperror.New(apperror.ErrInvalidData, apperror.Text{UZ: "Balans ruxsat etilgan chegaradan tashqarida", RU: "Баланс вне допустимого диапазона", EN: "The balance is out of range"})
	}
	if utf8.RuneCountInString(input.Reason) > maxAdjustmentReason {
		return BalanceAdjustment{}, apperror.ReasonTooLong(maxAdjustmentReason)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return BalanceAdjustment{}, fmt.Errorf("beginning balance adjustment: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	change, err := newMemberChange(ctx, tx, memberLookup{groupID: groupID, userID: userID}, actorID)
	if err != nil {
		return BalanceAdjustment{}, err
	}
	if !change.actor.isManager() {
		return BalanceAdjustment{}, apperror.ErrForbidden
	}
	if change.target.role != roleEmployee {
		return BalanceAdjustment{}, apperror.New(apperror.ErrRecordNotFound, apperror.Text{UZ: "Balans faqat xodimlarda bo'ladi", RU: "Баланс есть только у сотрудников", EN: "Only employees have a balance"})
	}
	memberID := change.target.memberID
	oldBalance, err := lockBalance(ctx, tx, groupID, memberID)
	if err != nil {
		return BalanceAdjustment{}, err
	}
	if input.NewBalanceUSD == oldBalance {
		return BalanceAdjustment{}, apperror.New(apperror.ErrInvalidData, apperror.Text{UZ: "Yangi balans hozirgi balansga teng", RU: "Новый баланс равен текущему", EN: "The new balance equals the current balance"})
	}
	now := s.now().UTC()
	_, err = tx.Exec(ctx, `
		UPDATE employee_balances SET balance_usd = $3, updated_at = $4
		WHERE group_id = $1 AND member_id = $2 AND deleted_at IS NULL
	`, groupID, memberID, input.NewBalanceUSD, now)
	if err != nil {
		return BalanceAdjustment{}, fmt.Errorf("updating employee balance: %w", err)
	}
	var adjustmentID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO balance_adjustments (
			group_id, member_id, old_balance_usd, new_balance_usd, amount_usd, reason, created_by, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8, $8)
		RETURNING id
	`, groupID, memberID, oldBalance, input.NewBalanceUSD, input.NewBalanceUSD-oldBalance, input.Reason, actorID, now).Scan(&adjustmentID)
	if err != nil {
		return BalanceAdjustment{}, fmt.Errorf("recording balance adjustment: %w", err)
	}
	adjustments, err := loadAdjustments(ctx, tx, adjustmentFilter{groupID: groupID, memberID: memberID, id: &adjustmentID, page: paging.Page{Limit: 1}})
	if err != nil {
		return BalanceAdjustment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BalanceAdjustment{}, fmt.Errorf("committing balance adjustment: %w", err)
	}
	return adjustments[0], nil
}

// lockBalance returns the member's balance under a row lock. Order completion
// updates the same row, so neither change can overwrite the other.
func lockBalance(ctx context.Context, q database.Querier, groupID, memberID uuid.UUID) (int64, error) {
	if err := ensureBalanceRow(ctx, q, groupID, memberID); err != nil {
		return 0, err
	}
	var balance int64
	err := q.QueryRow(ctx, `
		SELECT balance_usd FROM employee_balances
		WHERE group_id = $1 AND member_id = $2 AND deleted_at IS NULL
		FOR UPDATE
	`, groupID, memberID).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("locking employee balance: %w", err)
	}
	return balance, nil
}

// ListBalanceAdjustments returns a member's manual adjustments, newest first.
func (s *Service) ListBalanceAdjustments(ctx context.Context, actorID, groupID, userID uuid.UUID, page paging.Page) (BalanceHistory, error) {
	page, err := page.Valid()
	if err != nil {
		return BalanceHistory{}, err
	}
	a, err := loadActor(ctx, s.pool, groupID, actorID)
	if err != nil {
		return BalanceHistory{}, err
	}
	target, err := loadMember(ctx, s.pool, memberLookup{groupID: groupID, userID: userID})
	if err != nil {
		return BalanceHistory{}, err
	}
	if !a.canView(target) {
		return BalanceHistory{}, apperror.ErrRecordNotFound
	}
	adjustments, err := loadAdjustments(ctx, s.pool, adjustmentFilter{groupID: groupID, memberID: target.memberID, page: page})
	if err != nil {
		return BalanceHistory{}, err
	}
	history := BalanceHistory{
		Member:            BalanceMember{UserID: target.userID, FullName: target.fullName, AvatarURL: target.avatarURL, Role: target.role},
		CurrentBalanceUSD: target.balanceUSD,
		Adjustments:       adjustments,
	}
	if target.location != nil {
		history.Member.LocationName = target.location.Name
	}
	return history, nil
}

type adjustmentFilter struct {
	groupID, memberID uuid.UUID
	// id narrows the result to one adjustment.
	id   *uuid.UUID
	page paging.Page
}

func loadAdjustments(ctx context.Context, q database.Querier, f adjustmentFilter) ([]BalanceAdjustment, error) {
	rows, err := q.Query(ctx, `
		SELECT adjustments.id, adjustments.amount_usd, adjustments.old_balance_usd, adjustments.new_balance_usd,
		       COALESCE(adjustments.reason, ''), adjustments.created_by,
		       COALESCE(
		           NULLIF(btrim(concat_ws(' ', users.first_name, users.last_name)), ''),
		           NULLIF(users.name, ''), NULLIF(users.username, ''), ''
		       ),
		       adjustments.created_at
		FROM balance_adjustments adjustments
		JOIN users ON users.id = adjustments.created_by
		WHERE adjustments.group_id = $1
		  AND adjustments.member_id = $2
		  AND adjustments.deleted_at IS NULL
		  AND ($3::uuid IS NULL OR adjustments.id = $3)
		ORDER BY adjustments.created_at DESC, adjustments.id DESC
		LIMIT $4 OFFSET $5
	`, f.groupID, f.memberID, f.id, f.page.Limit, f.page.Offset)
	if err != nil {
		return nil, fmt.Errorf("listing balance adjustments: %w", err)
	}
	defer rows.Close()
	adjustments := make([]BalanceAdjustment, 0)
	for rows.Next() {
		var a BalanceAdjustment
		if err := rows.Scan(&a.ID, &a.AmountUSD, &a.OldBalanceUSD, &a.NewBalanceUSD, &a.Reason, &a.ChangedBy.UserID, &a.ChangedBy.Name, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning balance adjustment: %w", err)
		}
		a.Direction = "increase"
		if a.AmountUSD < 0 {
			a.Direction = "decrease"
		}
		adjustments = append(adjustments, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating balance adjustments: %w", err)
	}
	return adjustments, nil
}
