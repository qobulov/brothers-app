package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Options sizes the connection pool. On serverless hosts every running
// instance has its own pool, so MaxConns multiplied by the number of instances
// must stay under the database role's connection limit.
type Options struct {
	DSN      string
	MaxConns int
	MinConns int
}

func Connect(ctx context.Context, options Options) (*pgxpool.Pool, error) {
	config, err := poolConfig(options)
	if err != nil {
		return nil, err
	}
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

func poolConfig(options Options) (*pgxpool.Config, error) {
	if options.MaxConns < 1 || options.MinConns < 0 || options.MinConns > options.MaxConns {
		return nil, fmt.Errorf("invalid database pool size: max %d, min %d", options.MaxConns, options.MinConns)
	}
	config, err := pgxpool.ParseConfig(options.DSN)
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
	config.MaxConns = int32(options.MaxConns)
	config.MinConns = int32(options.MinConns)
	config.MaxConnLifetime = 5 * time.Minute
	// Idle connections are returned quickly so idle instances do not hold the
	// role's limited connections.
	config.MaxConnIdleTime = 30 * time.Second
	return config, nil
}
