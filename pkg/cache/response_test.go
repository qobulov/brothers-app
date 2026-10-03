package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/responses"
	"github.com/redis/go-redis/v9"
)

type memoryResponseStore struct {
	mu     sync.Mutex
	values map[string][]byte
}

func newMemoryResponseStore() *memoryResponseStore {
	return &memoryResponseStore{values: make(map[string][]byte)}
}

func (s *memoryResponseStore) Get(_ context.Context, key string) *redis.StringCmd {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	if !ok {
		return redis.NewStringResult("", redis.Nil)
	}
	return redis.NewStringResult(string(value), nil)
}

func (s *memoryResponseStore) Set(_ context.Context, key string, value interface{}, _ time.Duration) *redis.StatusCmd {
	s.mu.Lock()
	defer s.mu.Unlock()
	var encoded []byte
	switch value := value.(type) {
	case []byte:
		encoded = append([]byte(nil), value...)
	case string:
		encoded = []byte(value)
	default:
		encoded = []byte(fmt.Sprint(value))
	}
	s.values[key] = encoded
	return redis.NewStatusResult("OK", nil)
}

func (s *memoryResponseStore) Incr(_ context.Context, key string) *redis.IntCmd {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, _ := strconv.ParseInt(string(s.values[key]), 10, 64)
	value++
	s.values[key] = []byte(strconv.FormatInt(value, 10))
	return redis.NewIntResult(value, nil)
}

func TestResponseCacheCachesGETWithFreshMetadata(t *testing.T) {
	t.Parallel()
	store := newMemoryResponseStore()
	responseCache := NewResponseCache(store, time.Minute)
	app := fiber.New()
	responses.Middleware(app, "test")
	app.Use(responseCache.Cache())
	var calls atomic.Int32
	app.Get("/items", func(c *fiber.Ctx) error {
		return responses.Success(c, fiber.StatusOK, fiber.Map{"calls": calls.Add(1)}, responses.MessageRequestProcessed)
	})

	first := performRequest(t, app, fiber.MethodGet, "/items", "request-one")
	second := performRequest(t, app, fiber.MethodGet, "/items", "request-two")
	if calls.Load() != 1 {
		t.Fatalf("handler calls = %d, want 1", calls.Load())
	}
	if first.Header.Get("X-Cache") != "MISS" || second.Header.Get("X-Cache") != "HIT" {
		t.Fatalf("X-Cache = %q/%q, want MISS/HIT", first.Header.Get("X-Cache"), second.Header.Get("X-Cache"))
	}
	if requestID(t, first) != "request-one" || requestID(t, second) != "request-two" {
		t.Fatal("cached response replayed stale request metadata")
	}
}

func TestResponseCacheInvalidatesAfterMutation(t *testing.T) {
	t.Parallel()
	store := newMemoryResponseStore()
	responseCache := NewResponseCache(store, time.Minute)
	app := fiber.New()
	responses.Middleware(app, "test")
	app.Use(responseCache.Cache())
	var value atomic.Int32
	app.Get("/items", func(c *fiber.Ctx) error {
		return responses.Success(c, fiber.StatusOK, fiber.Map{"value": value.Load()}, responses.MessageRequestProcessed)
	})
	app.Post("/items", responseCache.Invalidate(), func(c *fiber.Ctx) error {
		value.Add(1)
		return responses.Success[any](c, fiber.StatusOK, nil, responses.MessageRequestProcessed)
	})

	first := performRequest(t, app, fiber.MethodGet, "/items", "request-one")
	before := responseValue(t, first)
	performRequest(t, app, fiber.MethodPost, "/items", "request-write")
	afterResponse := performRequest(t, app, fiber.MethodGet, "/items", "request-two")
	after := responseValue(t, afterResponse)
	if before != 0 || after != 1 || afterResponse.Header.Get("X-Cache") != "MISS" {
		t.Fatalf("values before/after = %d/%d, cache = %q", before, after, afterResponse.Header.Get("X-Cache"))
	}
}

func TestResponseCacheDoesNotCacheErrorsOrInvalidateFailedMutations(t *testing.T) {
	t.Parallel()
	store := newMemoryResponseStore()
	responseCache := NewResponseCache(store, time.Minute)
	app := fiber.New()
	responses.Middleware(app, "test")
	app.Use(responseCache.Cache())
	var calls atomic.Int32
	app.Get("/failure", func(c *fiber.Ctx) error {
		calls.Add(1)
		return responses.Failure(c, fiber.StatusBadRequest, 1400, "bad_request", "bad request", nil)
	})
	app.Post("/failure", responseCache.Invalidate(), func(c *fiber.Ctx) error {
		return responses.Failure(c, fiber.StatusBadRequest, 1400, "bad_request", "bad request", nil)
	})

	performRequest(t, app, fiber.MethodGet, "/failure", "request-one").Body.Close()
	performRequest(t, app, fiber.MethodGet, "/failure", "request-two").Body.Close()
	if calls.Load() != 2 {
		t.Fatalf("failed GET handler calls = %d, want 2", calls.Load())
	}
	performRequest(t, app, fiber.MethodPost, "/failure", "request-write").Body.Close()
	if _, ok := store.values[responseCacheVersion]; ok {
		t.Fatal("failed mutation invalidated the cache")
	}
}

func TestResponseCacheSeparatesUsers(t *testing.T) {
	t.Parallel()
	store := newMemoryResponseStore()
	responseCache := NewResponseCache(store, time.Minute)
	app := fiber.New()
	responses.Middleware(app, "test")
	app.Use(func(c *fiber.Ctx) error {
		userID, err := uuid.Parse(c.Get("X-Test-User"))
		if err != nil {
			return err
		}
		c.Locals("auth_user_id", userID)
		return c.Next()
	})
	app.Use(responseCache.Cache())
	app.Get("/me", func(c *fiber.Ctx) error {
		return responses.Success(c, fiber.StatusOK, fiber.Map{"user_id": c.Locals("auth_user_id")}, responses.MessageRequestProcessed)
	})

	firstID := uuid.New()
	secondID := uuid.New()
	first := performUserRequest(t, app, firstID)
	second := performUserRequest(t, app, secondID)
	if responseUserID(t, first) != firstID.String() || responseUserID(t, second) != secondID.String() {
		t.Fatal("cache mixed responses between authenticated users")
	}
}

func TestResponseCacheCoalescesConcurrentMisses(t *testing.T) {
	t.Parallel()
	store := newMemoryResponseStore()
	responseCache := NewResponseCache(store, time.Minute)
	app := fiber.New()
	responses.Middleware(app, "test")
	app.Use(responseCache.Cache())
	var calls atomic.Int32
	app.Get("/items", func(c *fiber.Ctx) error {
		calls.Add(1)
		time.Sleep(20 * time.Millisecond)
		return responses.Success(c, fiber.StatusOK, fiber.Map{"ok": true}, responses.MessageRequestProcessed)
	})

	const requests = 20
	var wg sync.WaitGroup
	errors := make(chan error, requests)
	wg.Add(requests)
	for index := range requests {
		go func() {
			defer wg.Done()
			request := httptest.NewRequest(fiber.MethodGet, "/items", nil)
			request.Header.Set("X-Request-ID", fmt.Sprintf("request-%02d", index))
			response, err := app.Test(request)
			if err == nil {
				err = response.Body.Close()
			}
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent request: %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("handler calls = %d, want 1", calls.Load())
	}
}

func performRequest(t *testing.T, app *fiber.App, method, path, requestID string) *http.Response {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	request.Header.Set("X-Request-ID", requestID)
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, path, err)
	}
	return response
}

func performUserRequest(t *testing.T, app *fiber.App, userID uuid.UUID) *http.Response {
	t.Helper()
	request := httptest.NewRequest(fiber.MethodGet, "/me", nil)
	request.Header.Set("X-Request-ID", "request-"+userID.String())
	request.Header.Set("X-Test-User", userID.String())
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("request user %s: %v", userID, err)
	}
	return response
}

func decodeEnvelope(t *testing.T, response *http.Response) responses.Envelope[map[string]any] {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	var envelope responses.Envelope[map[string]any]
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return envelope
}

func requestID(t *testing.T, response *http.Response) string {
	return decodeEnvelope(t, response).Meta.RequestID
}

func responseValue(t *testing.T, response *http.Response) int {
	return int(decodeEnvelope(t, response).Data["value"].(float64))
}

func responseUserID(t *testing.T, response *http.Response) string {
	return decodeEnvelope(t, response).Data["user_id"].(string)
}
