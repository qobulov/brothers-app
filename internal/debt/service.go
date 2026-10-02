package debt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qobulov/brothers-app/pkg/helpers"
)

type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, now: time.Now}
}

// querier is satisfied by both *pgxpool.Pool and pgx.Tx.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

const debtColumns = `
	id, direction, person_name, COALESCE(person_phone, ''), currency,
	original_amount, remaining_amount, status, created_at, completed_at`

func scanDebt(row pgx.Row) (Debt, error) {
	var d Debt
	err := row.Scan(
		&d.ID, &d.Direction, &d.PersonName, &d.PersonPhone, &d.Currency,
		&d.OriginalAmount, &d.RemainingAmount, &d.Status, &d.CreatedAt, &d.CompletedAt,
	)
	return d, err
}

func (s *Service) Create(ctx context.Context, ownerID uuid.UUID, input CreateInput) (Debt, error) {
	input, err := validCreateInput(input)
	if err != nil {
		return Debt{}, err
	}
	now := s.now().UTC()
	row := s.pool.QueryRow(ctx, `
		INSERT INTO debts (
			owner_user_id, direction, person_name, person_phone, currency,
			original_amount, remaining_amount, status, created_at, updated_at
		)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $6, 'active', $7, $7)
		RETURNING `+debtColumns,
		ownerID, input.Direction, input.PersonName, input.PersonPhone, input.Currency, input.Amount, now)
	created, err := scanDebt(row)
	if err != nil {
		return Debt{}, fmt.Errorf("creating debt: %w", err)
	}
	return created, nil
}

// Get returns one of the owner's debts. Someone else's debt is reported as not
// found, so its existence is not disclosed.
func (s *Service) Get(ctx context.Context, ownerID, debtID uuid.UUID) (Debt, error) {
	return loadDebt(ctx, s.pool, debtLookup{ownerID: ownerID, debtID: debtID})
}

type debtLookup struct {
	ownerID, debtID uuid.UUID
	// lock takes a row lock for the rest of the transaction.
	lock bool
}

func loadDebt(ctx context.Context, q querier, lookup debtLookup) (Debt, error) {
	query := `SELECT ` + debtColumns + `
		FROM debts
		WHERE id = $1 AND owner_user_id = $2 AND deleted_at IS NULL`
	if lookup.lock {
		query += " FOR UPDATE"
	}
	d, err := scanDebt(q.QueryRow(ctx, query, lookup.debtID, lookup.ownerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Debt{}, notFound()
	}
	if err != nil {
		return Debt{}, fmt.Errorf("loading debt: %w", err)
	}
	return d, nil
}

// List returns the owner's debts, newest first.
func (s *Service) List(ctx context.Context, ownerID uuid.UUID, input ListInput) ([]Debt, error) {
	input, err := validListInput(input)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+debtColumns+`
		FROM debts
		WHERE owner_user_id = $1
		  AND deleted_at IS NULL
		  AND ($2::text = '' OR direction = $2)
		  AND ($3::text = 'all' OR status = $3)
		  AND (
		      $4::text = ''
		      OR person_name ILIKE '%' || $4 || '%'
		      OR ($5::text <> '' AND COALESCE(person_phone, '') LIKE '%' || $5 || '%')
		  )
		ORDER BY created_at DESC, id DESC
		LIMIT $6 OFFSET $7
	`, ownerID, input.Direction, input.Status, helpers.EscapeLike(input.Query), phoneDigits(input.Query), input.Limit, input.Offset)
	if err != nil {
		return nil, fmt.Errorf("listing debts: %w", err)
	}
	defer rows.Close()
	debts := make([]Debt, 0)
	for rows.Next() {
		d, err := scanDebt(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning debt: %w", err)
		}
		debts = append(debts, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating debts: %w", err)
	}
	return debts, nil
}

// Summary totals the remaining amounts of active debts per currency and direction.
func (s *Service) Summary(ctx context.Context, ownerID uuid.UUID) (Summary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT currency, direction, COALESCE(SUM(remaining_amount), 0)::bigint
		FROM debts
		WHERE owner_user_id = $1 AND deleted_at IS NULL AND status = 'active'
		GROUP BY currency, direction
	`, ownerID)
	if err != nil {
		return Summary{}, fmt.Errorf("summarising debts: %w", err)
	}
	defer rows.Close()
	var summary Summary
	for rows.Next() {
		var currency, direction string
		var total int64
		if err := rows.Scan(&currency, &direction, &total); err != nil {
			return Summary{}, fmt.Errorf("scanning debt total: %w", err)
		}
		totals := &summary.USD
		if currency == CurrencyUZS {
			totals = &summary.UZS
		}
		if direction == DirectionIOwe {
			totals.IOwe = total
		} else {
			totals.TheyOweMe = total
		}
	}
	if err := rows.Err(); err != nil {
		return Summary{}, fmt.Errorf("iterating debt totals: %w", err)
	}
	return summary, nil
}

// Complete closes an active debt. The remaining amount is kept, so a closed
// debt still shows how much was never repaid.
func (s *Service) Complete(ctx context.Context, ownerID, debtID uuid.UUID) (Debt, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Debt{}, fmt.Errorf("beginning debt completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	d, err := loadDebt(ctx, tx, debtLookup{ownerID: ownerID, debtID: debtID, lock: true})
	if err != nil {
		return Debt{}, err
	}
	if d.Status != StatusActive {
		return Debt{}, alreadyCompleted()
	}
	if err := markCompleted(ctx, tx, debtID, s.now().UTC()); err != nil {
		return Debt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Debt{}, fmt.Errorf("committing debt completion: %w", err)
	}
	return s.Get(ctx, ownerID, debtID)
}

func markCompleted(ctx context.Context, q querier, debtID uuid.UUID, at time.Time) error {
	_, err := q.Exec(ctx, `
		UPDATE debts SET status = 'completed', completed_at = $2, updated_at = $2 WHERE id = $1
	`, debtID, at)
	if err != nil {
		return fmt.Errorf("completing debt: %w", err)
	}
	return nil
}

// Delete soft-deletes a debt, even one with repayments; it is a private record.
func (s *Service) Delete(ctx context.Context, ownerID, debtID uuid.UUID) error {
	now := s.now().UTC()
	result, err := s.pool.Exec(ctx, `
		UPDATE debts SET deleted_at = $3, updated_at = $3
		WHERE id = $1 AND owner_user_id = $2 AND deleted_at IS NULL
	`, debtID, ownerID, now)
	if err != nil {
		return fmt.Errorf("deleting debt: %w", err)
	}
	if result.RowsAffected() == 0 {
		return notFound()
	}
	return nil
}
