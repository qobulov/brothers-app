package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMemoryStoreReplacesPreviousSession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()
	userID := uuid.New()
	first := Data{UserID: userID, SessionID: uuid.New(), RefreshTokenHash: "first"}
	second := Data{UserID: userID, SessionID: uuid.New(), RefreshTokenHash: "second"}

	if err := store.Create(ctx, first, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, second, time.Hour); err != nil {
		t.Fatal(err)
	}
	active, err := store.Validate(ctx, userID, first.SessionID)
	if err != nil || active {
		t.Fatalf("old session active = %v, err = %v; want false, nil", active, err)
	}
	active, err = store.Validate(ctx, userID, second.SessionID)
	if err != nil || !active {
		t.Fatalf("new session active = %v, err = %v; want true, nil", active, err)
	}
	if _, err := store.Rotate(ctx, "first", "third", time.Hour); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rotating superseded refresh token = %v, want ErrNotFound", err)
	}
}

func TestMemoryStoreRotatesRefreshOnlyOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()
	data := Data{UserID: uuid.New(), SessionID: uuid.New(), RefreshTokenHash: "old"}
	if err := store.Create(ctx, data, time.Hour); err != nil {
		t.Fatal(err)
	}
	rotated, err := store.Rotate(ctx, "old", "new", time.Hour)
	if err != nil || rotated.SessionID != data.SessionID {
		t.Fatalf("Rotate() = %+v, %v", rotated, err)
	}
	if _, err := store.Rotate(ctx, "old", "another", time.Hour); !errors.Is(err, ErrNotFound) {
		t.Fatalf("reusing old refresh token = %v, want ErrNotFound", err)
	}
}
