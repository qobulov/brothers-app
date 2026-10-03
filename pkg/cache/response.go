package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/responses"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

const (
	responseCachePrefix   = "brothers:api-cache"
	responseCacheVersion  = responseCachePrefix + ":version"
	maxCachedResponseSize = 1 << 20
)

// ResponseStore is the subset of Redis used by the API response cache.
type ResponseStore interface {
	Get(context.Context, string) *redis.StringCmd
	Set(context.Context, string, interface{}, time.Duration) *redis.StatusCmd
	Incr(context.Context, string) *redis.IntCmd
}

// ResponseCache caches successful GET envelopes and invalidates them after
// successful mutations. Versioned keys make invalidation atomic across app
// instances without scanning or deleting Redis keys in the request path.
type ResponseCache struct {
	store ResponseStore
	ttl   time.Duration
	group singleflight.Group
}

func NewResponseCache(store ResponseStore, ttl time.Duration) *ResponseCache {
	return &ResponseCache{store: store, ttl: ttl}
}

type cachedEnvelope struct {
	Status  int             `json:"status"`
	Success bool            `json:"success"`
	Code    int             `json:"code"`
	Slug    string          `json:"slug"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// Cache must run after authentication middleware so private responses are
// keyed by the authenticated user and revoked sessions cannot use cached data.
func (r *ResponseCache) Cache() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if r == nil || r.store == nil || r.ttl <= 0 || c.Method() != fiber.MethodGet {
			return c.Next()
		}

		identity, ok := cacheIdentity(c)
		if !ok {
			return c.Next()
		}
		version, err := r.version(c.UserContext())
		if err != nil {
			slog.Warn("reading API cache version", "error", err)
			return c.Next()
		}
		key := responseKey(version, identity, c)
		if entry, found := r.load(c.UserContext(), key); found {
			return serveCached(c, entry, "HIT")
		}

		value, err, _ := r.group.Do(key, func() (any, error) {
			if entry, found := r.load(c.UserContext(), key); found {
				return entry, nil
			}
			if err := c.Next(); err != nil {
				return nil, err
			}
			entry, cacheable := captureResponse(c)
			if !cacheable {
				return nil, nil
			}
			encoded, err := json.Marshal(entry)
			if err != nil {
				return nil, nil
			}
			if err := r.store.Set(c.UserContext(), key, encoded, r.ttl).Err(); err != nil {
				slog.Warn("writing API response cache", "error", err)
			}
			return entry, nil
		})
		if err != nil {
			return err
		}
		if entry, ok := value.(cachedEnvelope); ok {
			return serveCached(c, entry, "MISS")
		}
		// The leader already produced a non-cacheable response. Followers have
		// no response yet and execute their own handler.
		if len(c.Response().Body()) == 0 {
			return c.Next()
		}
		return nil
	}
}

// Invalidate advances the shared generation after a successful data mutation.
func (r *ResponseCache) Invalidate() fiber.Handler {
	return func(c *fiber.Ctx) error {
		err := c.Next()
		if r == nil || r.store == nil || err != nil {
			return err
		}
		status := c.Response().StatusCode()
		if status < fiber.StatusOK || status >= fiber.StatusBadRequest {
			return nil
		}
		if cacheErr := r.store.Incr(c.UserContext(), responseCacheVersion).Err(); cacheErr != nil {
			// The database mutation already committed. Returning an error here
			// would encourage an unsafe retry; entries also have a short TTL.
			slog.Error("invalidating API response cache", "error", cacheErr)
		}
		return nil
	}
}

func (r *ResponseCache) version(ctx context.Context) (string, error) {
	version, err := r.store.Get(ctx, responseCacheVersion).Result()
	if errors.Is(err, redis.Nil) {
		return "0", nil
	}
	return version, err
}

func (r *ResponseCache) load(ctx context.Context, key string) (cachedEnvelope, bool) {
	value, err := r.store.Get(ctx, key).Bytes()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			slog.Warn("reading API response cache", "error", err)
		}
		return cachedEnvelope{}, false
	}
	var entry cachedEnvelope
	if err := json.Unmarshal(value, &entry); err != nil || !entry.Success {
		return cachedEnvelope{}, false
	}
	return entry, true
}

func cacheIdentity(c *fiber.Ctx) (string, bool) {
	if userID, ok := c.Locals("auth_user_id").(uuid.UUID); ok && userID != uuid.Nil {
		return userID.String(), true
	}
	// Never cache credentialed traffic unless authentication already established
	// the user identity. This prevents private data crossing security boundaries.
	if strings.TrimSpace(c.Get(fiber.HeaderAuthorization)) != "" {
		return "", false
	}
	return "public", true
}

func responseKey(version, identity string, c *fiber.Ctx) string {
	material := strings.Join([]string{
		version,
		identity,
		c.OriginalURL(),
		responses.Language(c),
		c.Get("Api-Version"),
	}, "\x00")
	sum := sha256.Sum256([]byte(material))
	return responseCachePrefix + ":response:" + hex.EncodeToString(sum[:])
}

func captureResponse(c *fiber.Ctx) (cachedEnvelope, bool) {
	status := c.Response().StatusCode()
	body := c.Response().Body()
	if status < fiber.StatusOK || status >= fiber.StatusMultipleChoices || len(body) == 0 || len(body) > maxCachedResponseSize {
		return cachedEnvelope{}, false
	}
	var response responses.Envelope[json.RawMessage]
	if err := json.Unmarshal(body, &response); err != nil || !response.Success {
		return cachedEnvelope{}, false
	}
	return cachedEnvelope{
		Status: status, Success: true, Code: response.Code, Slug: response.Slug,
		Message: response.Message, Data: response.Data,
	}, true
}

func serveCached(c *fiber.Ctx, entry cachedEnvelope, result string) error {
	body, err := json.Marshal(responses.Envelope[json.RawMessage]{
		Success: entry.Success,
		Code:    entry.Code,
		Slug:    entry.Slug,
		Message: entry.Message,
		Data:    entry.Data,
		Meta:    responses.Metadata(c),
	})
	if err != nil {
		return err
	}
	c.Set("X-Cache", result)
	c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSONCharsetUTF8)
	return c.Status(entry.Status).Send(body)
}
