package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

const testDatabaseLockID int64 = 0x62726f7468657273
const testDatabaseTimeout = 2 * time.Minute

func SetupTestDB(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	for _, path := range []string{".env.dev", "../../.env.dev", "../../../.env.dev"} {
		if _, err := os.Stat(path); err == nil {
			if err := godotenv.Load(path); err != nil {
				t.Fatalf("load test environment %s: %v", path, err)
			}
			break
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), testDatabaseTimeout)
	defer cancel()

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnv("DB_HOST", "localhost"), getEnv("DB_TEST_PORT", "5432"), getEnv("DB_TEST_USER", "postgres"),
		getEnv("DB_TEST_PASSWORD", ""), getEnv("DB_TEST_NAME", "test"),
	)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping test database: %v", err)
	}

	// Package tests run in separate processes and share the same test database.
	// Hold a session-level lock so another package cannot truncate these tables
	// while the current test is using them.
	lockConn, err := pool.Acquire(ctx)
	if err != nil {
		pool.Close()
		t.Fatalf("acquire test database lock connection: %v", err)
	}
	if _, err := lockConn.Exec(ctx, "SELECT pg_advisory_lock($1)", testDatabaseLockID); err != nil {
		lockConn.Release()
		pool.Close()
		t.Fatalf("lock test database: %v", err)
	}

	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			defer pool.Close()
			defer lockConn.Release()
			defer func() {
				unlockCtx, cancelUnlock := context.WithTimeout(context.Background(), testDatabaseTimeout)
				defer cancelUnlock()
				var unlocked bool
				if err := lockConn.QueryRow(unlockCtx, "SELECT pg_advisory_unlock($1)", testDatabaseLockID).Scan(&unlocked); err != nil {
					t.Errorf("unlock test database: %v", err)
				} else if !unlocked {
					t.Error("unlock test database: advisory lock was not held")
				}
			}()
			if err := cleanupTables(pool); err != nil {
				t.Errorf("clean test tables: %v", err)
			}
		})
	}
	t.Cleanup(cleanup)

	applyTestSchema(t, pool)
	if err := cleanupTables(pool); err != nil {
		t.Fatalf("clean test tables: %v", err)
	}
	return pool, cleanup
}

func applyTestSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var migrationPaths []string
	for _, pattern := range []string{"migrations/*.up.sql", "../../migrations/*.up.sql", "../../../migrations/*.up.sql"} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("find test migrations: %v", err)
		}
		if len(matches) > 0 {
			migrationPaths = matches
			break
		}
	}
	if len(migrationPaths) == 0 {
		t.Fatal("no test migrations found")
	}
	ctx, cancel := context.WithTimeout(context.Background(), testDatabaseTimeout)
	defer cancel()
	for _, path := range migrationPaths {
		schema, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read test migration %s: %v", path, err)
		}
		if _, err := pool.Exec(ctx, string(schema)); err != nil {
			t.Fatalf("apply test migration %s: %v", path, err)
		}
	}
}

func cleanupTables(pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), testDatabaseTimeout)
	defer cancel()
	_, err := pool.Exec(ctx, "TRUNCATE TABLE user_sessions, users, orders RESTART IDENTITY CASCADE")
	return err
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
