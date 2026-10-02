package database

import (
	"context"
	"testing"
)

func TestMigrationsKeepUsernameCase(t *testing.T) {
	pool, cleanup := SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `INSERT INTO users (username, language) VALUES ('Abror', 'uz')`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	applyTestSchema(t, pool)

	var username string
	if err := pool.QueryRow(ctx, `SELECT username FROM users`).Scan(&username); err != nil {
		t.Fatalf("read username: %v", err)
	}
	if username != "Abror" {
		t.Fatalf("username = %q, want Abror unchanged", username)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (username, language) VALUES ('abror', 'uz')`); err == nil {
		t.Fatal("abror must be rejected while Abror exists: uniqueness ignores case")
	}
}
