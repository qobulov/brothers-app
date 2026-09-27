// Package session keeps authentication session state outside PostgreSQL.
package session

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var ErrNotFound = errors.New("session not found")

// Data is all short-lived server-side session state. It is intentionally kept
// in Redis only; no PostgreSQL session row is created.
type Data struct {
	UserID           uuid.UUID `json:"user_id"`
	SessionID        uuid.UUID `json:"session_id"`
	RefreshTokenHash string    `json:"refresh_token_hash"`
}

type Store interface {
	Create(context.Context, Data, time.Duration) error
	Validate(context.Context, uuid.UUID, uuid.UUID) (bool, error)
	Rotate(context.Context, string, string, time.Duration) (Data, error)
	Revoke(context.Context, uuid.UUID, uuid.UUID) (bool, error)
	RevokeUser(context.Context, uuid.UUID) error
}

type RedisStore struct {
	client redis.Cmdable
	prefix string
}

func NewRedisStore(client redis.Cmdable) *RedisStore {
	return &RedisStore{client: client, prefix: "brothers:auth"}
}

func (s *RedisStore) Create(ctx context.Context, data Data, ttl time.Duration) error {
	if err := validateData(data, ttl); err != nil {
		return err
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encoding session: %w", err)
	}
	locator, err := json.Marshal(locator{UserID: data.UserID, SessionID: data.SessionID})
	if err != nil {
		return fmt.Errorf("encoding refresh session locator: %w", err)
	}
	const script = `
local previous = redis.call('GET', KEYS[1])
if previous then
  local old = cjson.decode(previous)
  if old.refresh_token_hash then redis.call('DEL', ARGV[4] .. old.refresh_token_hash) end
end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[3])
redis.call('SET', KEYS[2], ARGV[2], 'PX', ARGV[3])
return 1`
	if err := s.client.Eval(ctx, script, []string{s.userKey(data.UserID), s.refreshKey(data.RefreshTokenHash)}, encoded, locator, ttl.Milliseconds(), s.refreshPrefix()).Err(); err != nil {
		return fmt.Errorf("saving redis session: %w", err)
	}
	return nil
}

func (s *RedisStore) Validate(ctx context.Context, userID, sessionID uuid.UUID) (bool, error) {
	data, err := s.loadUser(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare([]byte(data.SessionID.String()), []byte(sessionID.String())) == 1, nil
}

// Rotate invalidates the supplied refresh token exactly once and keeps the
// same session ID, so existing access tokens remain valid until expiry.
func (s *RedisStore) Rotate(ctx context.Context, oldHash, newHash string, ttl time.Duration) (Data, error) {
	if oldHash == "" || newHash == "" || ttl <= 0 {
		return Data{}, ErrNotFound
	}
	value, err := s.client.Get(ctx, s.refreshKey(oldHash)).Result()
	if errors.Is(err, redis.Nil) {
		return Data{}, ErrNotFound
	}
	if err != nil {
		return Data{}, fmt.Errorf("loading refresh session: %w", err)
	}
	var ref locator
	if err := json.Unmarshal([]byte(value), &ref); err != nil || ref.UserID == uuid.Nil || ref.SessionID == uuid.Nil {
		return Data{}, ErrNotFound
	}
	data, err := s.loadUser(ctx, ref.UserID)
	if errors.Is(err, ErrNotFound) {
		return Data{}, ErrNotFound
	}
	if err != nil {
		return Data{}, err
	}
	if data.SessionID != ref.SessionID || subtle.ConstantTimeCompare([]byte(data.RefreshTokenHash), []byte(oldHash)) != 1 {
		return Data{}, ErrNotFound
	}
	data.RefreshTokenHash = newHash
	encoded, err := json.Marshal(data)
	if err != nil {
		return Data{}, fmt.Errorf("encoding rotated session: %w", err)
	}
	newLocator, err := json.Marshal(ref)
	if err != nil {
		return Data{}, fmt.Errorf("encoding rotated refresh locator: %w", err)
	}
	const script = `
local refresh = redis.call('GET', KEYS[1])
local current = redis.call('GET', KEYS[2])
if not refresh or not current then return 0 end
local session = cjson.decode(current)
if session.refresh_token_hash ~= ARGV[1] then return 0 end
redis.call('DEL', KEYS[1])
redis.call('SET', KEYS[2], ARGV[2], 'PX', ARGV[4])
redis.call('SET', KEYS[3], ARGV[3], 'PX', ARGV[4])
return 1`
	rotated, err := s.client.Eval(ctx, script, []string{s.refreshKey(oldHash), s.userKey(ref.UserID), s.refreshKey(newHash)}, oldHash, encoded, newLocator, ttl.Milliseconds()).Int()
	if err != nil {
		return Data{}, fmt.Errorf("rotating redis session: %w", err)
	}
	if rotated != 1 {
		return Data{}, ErrNotFound
	}
	return data, nil
}

func (s *RedisStore) Revoke(ctx context.Context, userID, sessionID uuid.UUID) (bool, error) {
	const script = `
local current = redis.call('GET', KEYS[1])
if not current then return 0 end
local session = cjson.decode(current)
if session.session_id ~= ARGV[1] then return 0 end
if session.refresh_token_hash then redis.call('DEL', ARGV[2] .. session.refresh_token_hash) end
redis.call('DEL', KEYS[1])
return 1`
	revoked, err := s.client.Eval(ctx, script, []string{s.userKey(userID)}, sessionID.String(), s.refreshPrefix()).Int()
	if err != nil {
		return false, fmt.Errorf("revoking redis session: %w", err)
	}
	return revoked == 1, nil
}

func (s *RedisStore) RevokeUser(ctx context.Context, userID uuid.UUID) error {
	data, err := s.loadUser(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = s.Revoke(ctx, userID, data.SessionID)
	return err
}

func (s *RedisStore) loadUser(ctx context.Context, userID uuid.UUID) (Data, error) {
	value, err := s.client.Get(ctx, s.userKey(userID)).Result()
	if errors.Is(err, redis.Nil) {
		return Data{}, ErrNotFound
	}
	if err != nil {
		return Data{}, fmt.Errorf("loading redis session: %w", err)
	}
	var data Data
	if err := json.Unmarshal([]byte(value), &data); err != nil || data.UserID != userID || data.SessionID == uuid.Nil {
		return Data{}, ErrNotFound
	}
	return data, nil
}

func validateData(data Data, ttl time.Duration) error {
	if data.UserID == uuid.Nil || data.SessionID == uuid.Nil || data.RefreshTokenHash == "" || ttl <= 0 {
		return errors.New("invalid session data")
	}
	return nil
}

func (s *RedisStore) userKey(userID uuid.UUID) string {
	return s.prefix + ":session:" + userID.String()
}
func (s *RedisStore) refreshKey(hash string) string { return s.refreshPrefix() + hash }
func (s *RedisStore) refreshPrefix() string         { return s.prefix + ":refresh:" }

type locator struct {
	UserID    uuid.UUID `json:"user_id"`
	SessionID uuid.UUID `json:"session_id"`
}

// MemoryStore is deterministic test wiring. Production must use RedisStore.
type MemoryStore struct {
	mu    sync.Mutex
	users map[uuid.UUID]memoryEntry
	keys  map[string]uuid.UUID
}

type memoryEntry struct {
	data      Data
	expiresAt time.Time
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{users: make(map[uuid.UUID]memoryEntry), keys: make(map[string]uuid.UUID)}
}

func (s *MemoryStore) Create(_ context.Context, data Data, ttl time.Duration) error {
	if err := validateData(data, ttl); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remove(data.UserID)
	s.users[data.UserID] = memoryEntry{data: data, expiresAt: time.Now().Add(ttl)}
	s.keys[data.RefreshTokenHash] = data.UserID
	return nil
}
func (s *MemoryStore) Validate(_ context.Context, userID, sessionID uuid.UUID) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.users[userID]
	if !ok || time.Now().After(entry.expiresAt) {
		s.remove(userID)
		return false, nil
	}
	return entry.data.SessionID == sessionID, nil
}
func (s *MemoryStore) Rotate(_ context.Context, oldHash, newHash string, ttl time.Duration) (Data, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	userID, ok := s.keys[oldHash]
	if !ok {
		return Data{}, ErrNotFound
	}
	entry, ok := s.users[userID]
	if !ok || time.Now().After(entry.expiresAt) || entry.data.RefreshTokenHash != oldHash {
		s.remove(userID)
		return Data{}, ErrNotFound
	}
	delete(s.keys, oldHash)
	entry.data.RefreshTokenHash = newHash
	entry.expiresAt = time.Now().Add(ttl)
	s.users[userID] = entry
	s.keys[newHash] = userID
	return entry.data, nil
}
func (s *MemoryStore) Revoke(_ context.Context, userID, sessionID uuid.UUID) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.users[userID]
	if !ok || entry.data.SessionID != sessionID {
		return false, nil
	}
	s.remove(userID)
	return true, nil
}
func (s *MemoryStore) RevokeUser(_ context.Context, userID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remove(userID)
	return nil
}
func (s *MemoryStore) remove(userID uuid.UUID) {
	if entry, ok := s.users[userID]; ok {
		delete(s.keys, entry.data.RefreshTokenHash)
		delete(s.users, userID)
	}
}
