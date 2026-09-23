package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var DB *pgxpool.Pool

func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parsing database config: %w", err)
	}
	config.MaxConns = 25
	config.MinConns = 2
	config.MaxConnLifetime = 5 * time.Minute
	config.MaxConnIdleTime = time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("creating database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}
	DB = pool
	log.Println("✅ Database connected")
	return pool, nil
}

// Close closes the underlying sql.DB connection pool
func Close() error {
	fmt.Println("🔒 Closing Database connection...")
	if DB != nil {
		DB.Close()
	}
	return nil
}
