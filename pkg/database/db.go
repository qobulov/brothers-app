package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parsing database config: %w", err)
	}
	if config.ConnConfig.RuntimeParams == nil {
		config.ConnConfig.RuntimeParams = make(map[string]string)
	}
	// PostgreSQL timestamptz values are persisted as UTC instants. Pin every
	// connection to UTC so database-generated values (for example now()) and
	// scanned time.Time values consistently use RFC3339's Z offset.
	config.ConnConfig.RuntimeParams["TimeZone"] = "UTC"
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
	log.Println("✅ Database connected")
	return pool, nil
}
