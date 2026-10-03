package cache

import (
	"context"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"
)

// TestClient returns a Redis client on its own logical database, emptied for
// the test. Packages are tested in parallel, so each one passes a different
// db number to keep their keys apart. Without Redis the test is skipped.
func TestClient(t *testing.T, db int) *redis.Client {
	t.Helper()
	url := os.Getenv("REDIS_TEST_URL")
	if url == "" {
		url = "redis://localhost:6379/15"
	}
	options, err := redis.ParseURL(url)
	if err != nil {
		t.Fatalf("parse REDIS_TEST_URL: %v", err)
	}
	options.DB = db
	client := redis.NewClient(options)
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Skipf("redis is not available at %s: %v", url, err)
	}
	if err := client.FlushDB(context.Background()).Err(); err != nil {
		t.Fatalf("flush test redis db %d: %v", db, err)
	}
	return client
}
