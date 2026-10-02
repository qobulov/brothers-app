package database

import (
	"context"
	"testing"
)

// Debts are independent of orders: no column may reference them.
func TestDebtsHaveNoOrderReference(t *testing.T) {
	pool, cleanup := SetupTestDB(t)
	defer cleanup()

	var orderColumns int
	err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'debts' AND column_name LIKE '%order%'
	`).Scan(&orderColumns)
	if err != nil {
		t.Fatalf("read debts columns: %v", err)
	}
	if orderColumns != 0 {
		t.Fatalf("debts has %d order columns, want none", orderColumns)
	}
}
