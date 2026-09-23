package database

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func SetupTestDB(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	for _, path := range []string{".env.dev", "../../.env.dev", "../../../.env.dev"} {
		if _, err := os.Stat(path); err == nil {
			_ = godotenv.Load(path)
			break
		}
	}
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnv("DB_HOST", "localhost"), getEnv("DB_TEST_PORT", "5432"), getEnv("DB_TEST_USER", "postgres"),
		getEnv("DB_TEST_PASSWORD", ""), getEnv("DB_TEST_NAME", "test"),
	)
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Fatalf("ping test database: %v", err)
	}
	applyTestSchema(t, pool)
	cleanupTables(t, pool)
	return pool, func() {
		cleanupTables(t, pool)
		pool.Close()
	}
}

func applyTestSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var schema []byte
	var err error
	for _, path := range []string{"migrations/000001_auth_foundation.up.sql", "../../migrations/000001_auth_foundation.up.sql", "../../../migrations/000001_auth_foundation.up.sql"} {
		schema, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("read test schema: %v", err)
	}
	if _, err := pool.Exec(context.Background(), string(schema)); err != nil {
		t.Fatalf("apply test schema: %v", err)
	}
}

func cleanupTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "TRUNCATE TABLE user_sessions, users, orders RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("clean test tables: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
