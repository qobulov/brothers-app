package debt

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/apperror"
)

// Repay records a partial repayment. The debt row is locked first, so two
// repayments at the same moment cannot together exceed the remaining amount.
// A repayment that brings the remaining amount to zero completes the debt.
func (s *Service) Repay(ctx context.Context, ownerID, debtID uuid.UUID, amount int64) (Debt, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Debt{}, fmt.Errorf("beginning debt repayment: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	d, err := loadDebt(ctx, tx, debtLookup{ownerID: ownerID, debtID: debtID, lock: true})
	if err != nil {
		return Debt{}, err
	}
	if d.Status != StatusActive {
		return Debt{}, alreadyCompleted()
	}
	if err := validRepayment(amount, d); err != nil {
		return Debt{}, err
	}
	now := s.now().UTC()
	_, err = tx.Exec(ctx, `
		INSERT INTO debt_repayments (debt_id, amount, created_at, updated_at) VALUES ($1, $2, $3, $3)
	`, debtID, amount, now)
	if err != nil {
		return Debt{}, fmt.Errorf("recording debt repayment: %w", err)
	}
	_, err = tx.Exec(ctx, `
		UPDATE debts SET remaining_amount = remaining_amount - $2, updated_at = $3 WHERE id = $1
	`, debtID, amount, now)
	if err != nil {
		return Debt{}, fmt.Errorf("reducing debt: %w", err)
	}
	if amount == d.RemainingAmount {
		if err := markCompleted(ctx, tx, debtID, now); err != nil {
			return Debt{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Debt{}, fmt.Errorf("committing debt repayment: %w", err)
	}
	return s.Get(ctx, ownerID, debtID)
}

func validRepayment(amount int64, d Debt) error {
	if amount < 1 {
		return apperror.New(apperror.ErrInvalidData, apperror.Text{
			UZ: "To'lov summasi 0 dan katta bo'lishi kerak",
			RU: "Сумма платежа должна быть больше 0",
			EN: "The repayment must be greater than 0",
		})
	}
	if amount > d.RemainingAmount {
		remaining := apperror.FormatNumber(d.RemainingAmount)
		return apperror.New(apperror.ErrInvalidData, apperror.Text{
			UZ: fmt.Sprintf("To'lov qolgan summadan (%s %s) oshmasligi kerak", remaining.UZ, d.Currency),
			RU: fmt.Sprintf("Платёж не может превышать остаток (%s %s)", remaining.RU, d.Currency),
			EN: fmt.Sprintf("The repayment cannot exceed the remaining %s %s", remaining.EN, d.Currency),
		})
	}
	return nil
}

// ListRepayments returns a debt's repayments, newest first.
func (s *Service) ListRepayments(ctx context.Context, ownerID, debtID uuid.UUID, page Page) ([]Repayment, error) {
	page, err := validPage(page)
	if err != nil {
		return nil, err
	}
	if _, err := s.Get(ctx, ownerID, debtID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, amount, created_at
		FROM debt_repayments
		WHERE debt_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`, debtID, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("listing debt repayments: %w", err)
	}
	defer rows.Close()
	repayments := make([]Repayment, 0)
	for rows.Next() {
		var r Repayment
		if err := rows.Scan(&r.ID, &r.Amount, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning debt repayment: %w", err)
		}
		repayments = append(repayments, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating debt repayments: %w", err)
	}
	return repayments, nil
}
