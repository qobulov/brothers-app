package database

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestOrdersMigrationReplacesLegacyShape(t *testing.T) {
	pool, cleanup := SetupTestDB(t)
	defer cleanup()

	var legacyColumns, newColumns int
	err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FILTER (WHERE column_name = 'total'),
		       COUNT(*) FILTER (WHERE column_name IN ('giver_member_id', 'receiver_member_id', 'amount_usd', 'fee_uzs', 'status'))
		FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'orders'
	`).Scan(&legacyColumns, &newColumns)
	if err != nil {
		t.Fatalf("read orders columns: %v", err)
	}
	if legacyColumns != 0 || newColumns != 5 {
		t.Fatalf("orders columns: legacy=%d new=%d, want 0 and 5", legacyColumns, newColumns)
	}
}

// make migrate re-applies every file, so migration 14 must never delete real orders.
func TestReapplyingMigrationsKeepsOrders(t *testing.T) {
	pool, cleanup := SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	ownerID, giverID, receiverID, groupID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO users (id, username, language) VALUES ($1, 'owner', 'uz'), ($2, 'giver', 'uz'), ($3, 'receiver', 'uz')`, []any{ownerID, giverID, receiverID}},
		{`INSERT INTO groups (id, name, created_by) VALUES ($1, 'Brothers', $2)`, []any{groupID, ownerID}},
		{`INSERT INTO group_members (group_id, user_id, username, role)
		  VALUES ($1, $2, 'giver', 'employee'), ($1, $3, 'receiver', 'employee')`, []any{groupID, giverID, receiverID}},
		{`INSERT INTO customers (group_id, phone) VALUES ($1, '+998901111111'), ($1, '+998902222222')`, []any{groupID}},
		{`INSERT INTO orders (group_id, created_by, giver_member_id, receiver_member_id, giver_customer_id, receiver_customer_id, amount_usd)
		  SELECT $1, $2,
		         (SELECT id FROM group_members WHERE user_id = $3),
		         (SELECT id FROM group_members WHERE user_id = $4),
		         (SELECT id FROM customers WHERE phone = '+998901111111'),
		         (SELECT id FROM customers WHERE phone = '+998902222222'),
		         7000`, []any{groupID, ownerID, giverID, receiverID}},
	} {
		if _, err := pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seed order: %v", err)
		}
	}

	applyTestSchema(t, pool)

	var orders int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM orders`).Scan(&orders); err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if orders != 1 {
		t.Fatalf("orders after re-applying migrations = %d, want 1", orders)
	}
}
