package service

import (
	"context"
	"testing"

	dto "github.com/qobulov/brothers-app/internal/auth/dto"
	"github.com/qobulov/brothers-app/internal/auth/session"
	"github.com/qobulov/brothers-app/pkg/config"
	"github.com/qobulov/brothers-app/pkg/database"
)

func (f *emailFixture) available(username string) bool {
	f.t.Helper()
	result, err := f.service.CheckUsername(context.Background(), username)
	if err != nil {
		f.t.Fatalf("check username %q: %v", username, err)
	}
	return result.Available
}

func TestCheckUsername_AnswersFromCache(t *testing.T) {
	f := newEmailFixture(t)
	if !f.available("fresh_name") {
		t.Fatal("an unused username must be available")
	}

	// Taken behind the service's back: the cached answer is served until it expires.
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO users (username, language) VALUES ('fresh_name', 'uz')`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if !f.available("FRESH_NAME") {
		t.Fatal("the cached answer must be reused, regardless of case")
	}
	if f.available("other_taken") != true {
		t.Fatal("a different username has its own cache entry")
	}
}

func TestCheckUsername_CacheIsClearedWhenUsernameIsTaken(t *testing.T) {
	f := newEmailFixture(t)
	ctx := context.Background()

	if !f.available("New_Profile") {
		t.Fatal("want available before the change")
	}
	userID := f.user("profile@example.com")
	username := "New_Profile"
	if _, err := f.service.UpdateCurrentUser(ctx, userID, dto.UpdateProfileRequest{Username: &username}); err != nil {
		t.Fatalf("change username: %v", err)
	}
	if f.available("new_profile") {
		t.Fatal("a username taken through the profile must be reported taken immediately")
	}

	if !f.available("registered1") {
		t.Fatal("want available before registration")
	}
	_, err := f.service.Register(ctx, dto.RegisterRequest{
		Email: "registered@example.com", Username: "Registered1", FirstName: "New", Password: "StrongPass123", OTPCode: defaultOTP,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if f.available("registered1") {
		t.Fatal("a username taken by registration must be reported taken immediately")
	}
}

// Without Redis the check still works; it just asks the database every time.
func TestCheckUsername_WorksWithoutCache(t *testing.T) {
	pool, cleanup := database.SetupTestDB(t)
	t.Cleanup(cleanup)
	service := New(pool, nil, session.NewMemoryStore(), &config.Config{}, nil)

	result, err := service.CheckUsername(context.Background(), "no_cache_name")
	if err != nil || !result.Available {
		t.Fatalf("check without cache = %+v, %v", result, err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO users (username, language) VALUES ('no_cache_name', 'uz')`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	result, err = service.CheckUsername(context.Background(), "no_cache_name")
	if err != nil || result.Available {
		t.Fatalf("check without cache after insert = %+v, %v; want taken", result, err)
	}
}
