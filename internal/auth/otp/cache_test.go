package otp

import (
	"context"
	"testing"
	"time"

	"github.com/qobulov/brothers-app/pkg/cache"
)

func TestIncrement_CountsInsideOneWindow(t *testing.T) {
	client := cache.TestClient(t, 14)
	counter := NewCache(client)
	ctx := context.Background()

	for want := int64(1); want <= 3; want++ {
		got, err := counter.Increment(ctx, "login:203.0.113.7", time.Minute)
		if err != nil {
			t.Fatalf("increment: %v", err)
		}
		if got != want {
			t.Fatalf("count = %d, want %d", got, want)
		}
	}
	if other, _ := counter.Increment(ctx, "login:203.0.113.8", time.Minute); other != 1 {
		t.Fatalf("another name must have its own counter, got %d", other)
	}

	// Later hits must not extend the window, or a steady client is never unblocked.
	ttl, err := client.PTTL(ctx, counter.key("login:203.0.113.7")).Result()
	if err != nil {
		t.Fatalf("read ttl: %v", err)
	}
	if ttl <= 0 || ttl > time.Minute {
		t.Fatalf("ttl = %s, want within one minute", ttl)
	}
}

func TestIncrement_StartsOverWhenWindowEnds(t *testing.T) {
	counter := NewCache(cache.TestClient(t, 14))
	ctx := context.Background()

	if _, err := counter.Increment(ctx, "short", 50*time.Millisecond); err != nil {
		t.Fatalf("increment: %v", err)
	}
	time.Sleep(120 * time.Millisecond)
	got, err := counter.Increment(ctx, "short", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("increment: %v", err)
	}
	if got != 1 {
		t.Fatalf("count after the window = %d, want 1", got)
	}
}
