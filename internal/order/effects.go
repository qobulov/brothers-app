package order

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/database"
)

// tashkentZone decides which month a profit belongs to. It is a fixed UTC+5
// zone: Uzbekistan has not observed DST since 1992, and a fixed zone avoids
// depending on tzdata in the runtime image.
var tashkentZone = time.FixedZone("Asia/Tashkent", 5*60*60)

// completeOrder marks the order completed and applies its money effects: the
// giver took cash from a customer (+amount), the receiver paid one out
// (-amount), and each party's own fee goes to their monthly profit.
func completeOrder(ctx context.Context, q database.Querier, st settlement, amountUSD int64) error {
	result, err := q.Exec(ctx, `
		UPDATE orders
		SET status = 'completed', completed_at = $2, updated_at = $2
		WHERE id = $1 AND status = 'pending'
	`, st.orderID, st.at)
	if err != nil {
		return fmt.Errorf("completing order: %w", err)
	}
	if result.RowsAffected() != 1 {
		return notPending(StatusCompleted)
	}
	giver := balanceChange{groupID: st.groupID, memberID: st.parties.giverMemberID, amountUSD: amountUSD, at: st.at}
	if err := addBalance(ctx, q, giver); err != nil {
		return err
	}
	receiver := balanceChange{groupID: st.groupID, memberID: st.parties.receiverMemberID, amountUSD: -amountUSD, at: st.at}
	if err := addBalance(ctx, q, receiver); err != nil {
		return err
	}
	for _, row := range st.confirmations {
		if row.feeUZS == 0 {
			continue
		}
		change := profitChange{groupID: st.groupID, memberID: row.memberID, feeUZS: row.feeUZS, at: st.at}
		if err := addProfit(ctx, q, change); err != nil {
			return err
		}
	}
	return nil
}

type balanceChange struct {
	groupID, memberID uuid.UUID
	amountUSD         int64
	at                time.Time
}

func addBalance(ctx context.Context, q database.Querier, change balanceChange) error {
	_, err := q.Exec(ctx, `
		INSERT INTO employee_balances (group_id, member_id, balance_usd, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $4)
		ON CONFLICT (group_id, member_id) WHERE deleted_at IS NULL
		DO UPDATE SET balance_usd = employee_balances.balance_usd + EXCLUDED.balance_usd,
		              updated_at = EXCLUDED.updated_at
	`, change.groupID, change.memberID, change.amountUSD, change.at)
	if err != nil {
		return fmt.Errorf("updating employee balance: %w", err)
	}
	return nil
}

type profitChange struct {
	groupID, memberID uuid.UUID
	feeUZS            int64
	at                time.Time
}

func addProfit(ctx context.Context, q database.Querier, change profitChange) error {
	local := change.at.In(tashkentZone)
	_, err := q.Exec(ctx, `
		INSERT INTO member_profit_periods (group_id, member_id, year, month, profit_uzs, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $6)
		ON CONFLICT (group_id, member_id, year, month) WHERE deleted_at IS NULL
		DO UPDATE SET profit_uzs = member_profit_periods.profit_uzs + EXCLUDED.profit_uzs,
		              updated_at = EXCLUDED.updated_at
	`, change.groupID, change.memberID, local.Year(), int(local.Month()), change.feeUZS, change.at)
	if err != nil {
		return fmt.Errorf("updating member profit: %w", err)
	}
	return nil
}

// reverseCompletion undoes completeOrder's money effects with opposite
// entries. Profit is reversed in the cancellation's month, so that month's
// profit may go negative; the month it was earned stays untouched.
func reverseCompletion(ctx context.Context, q database.Querier, st settlement) error {
	if len(st.confirmations) != 2 {
		return fmt.Errorf("reversing order: want 2 confirmations, have %d", len(st.confirmations))
	}
	amountUSD := st.confirmations[0].amountUSD
	giver := balanceChange{groupID: st.groupID, memberID: st.parties.giverMemberID, amountUSD: -amountUSD, at: st.at}
	if err := addBalance(ctx, q, giver); err != nil {
		return err
	}
	receiver := balanceChange{groupID: st.groupID, memberID: st.parties.receiverMemberID, amountUSD: amountUSD, at: st.at}
	if err := addBalance(ctx, q, receiver); err != nil {
		return err
	}
	for _, row := range st.confirmations {
		if row.feeUZS == 0 {
			continue
		}
		change := profitChange{groupID: st.groupID, memberID: row.memberID, feeUZS: -row.feeUZS, at: st.at}
		if err := addProfit(ctx, q, change); err != nil {
			return err
		}
	}
	return nil
}
