// Package cache owns connections to ephemeral shared storage.
package cache

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// ConnectRedis creates and verifies a Redis client from a Redis URL.
func ConnectRedis(ctx context.Context, redisURL string) (*redis.Client, error) {
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parsing redis url: %w", err)
	}

	client := redis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("pinging redis: %w", err)
	}

	return client, nil
}
