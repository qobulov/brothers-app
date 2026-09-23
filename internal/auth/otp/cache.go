// Package otp stores short-lived OTP hashes in Redis.
package otp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrNotFound = errors.New("otp not found")

type Cache struct {
	client redis.Cmdable
	prefix string
}

func NewCache(client redis.Cmdable) *Cache {
	return &Cache{client: client, prefix: "brothers:otp"}
}

// Set stores an OTP hash until it expires. The plaintext OTP is never cached.
func (c *Cache) Set(ctx context.Context, recipient, otpHash string, ttl time.Duration) error {
	if ttl <= 0 {
		return fmt.Errorf("otp ttl must be positive")
	}
	if err := c.client.Set(ctx, c.key(recipient), otpHash, ttl).Err(); err != nil {
		return fmt.Errorf("caching otp: %w", err)
	}
	return nil
}

// Get returns the cached OTP hash for a recipient.
func (c *Cache) Get(ctx context.Context, recipient string) (string, error) {
	value, err := c.client.Get(ctx, c.key(recipient)).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("loading otp: %w", err)
	}
	return value, nil
}

// Take atomically returns and deletes a cached value.
func (c *Cache) Take(ctx context.Context, recipient string) (string, error) {
	value, err := c.client.GetDel(ctx, c.key(recipient)).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("consuming otp value: %w", err)
	}
	return value, nil
}

// Verify atomically compares and consumes an OTP hash. Failed attempts are
// counted in Redis and the OTP is invalidated after maxAttempts.
func (c *Cache) Verify(ctx context.Context, recipient, expectedHash string, maxAttempts int) (bool, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	const script = `
local value = redis.call('GET', KEYS[1])
if not value then return -1 end
if value == ARGV[1] then
  redis.call('DEL', KEYS[1], KEYS[2])
  return 1
end
local attempts = redis.call('INCR', KEYS[2])
local ttl = redis.call('PTTL', KEYS[1])
if ttl > 0 then redis.call('PEXPIRE', KEYS[2], ttl) end
if attempts >= tonumber(ARGV[2]) then redis.call('DEL', KEYS[1], KEYS[2]) end
return 0`
	codeKey := c.key(recipient)
	result, err := c.client.Eval(ctx, script, []string{codeKey, codeKey + ":attempts"}, expectedHash, maxAttempts).Int64()
	if err != nil {
		return false, fmt.Errorf("verifying otp: %w", err)
	}
	return result == 1, nil
}

// Reserve creates a key only when it does not exist. It is used for resend
// cooldowns so concurrent requests cannot both send an OTP.
func (c *Cache) Reserve(ctx context.Context, recipient string, ttl time.Duration) (bool, error) {
	if ttl <= 0 {
		return false, fmt.Errorf("otp ttl must be positive")
	}
	reserved, err := c.client.SetNX(ctx, c.key(recipient), "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("reserving otp cooldown: %w", err)
	}
	return reserved, nil
}

// Delete consumes the OTP after successful verification.
func (c *Cache) Delete(ctx context.Context, recipient string) error {
	if err := c.client.Del(ctx, c.key(recipient)).Err(); err != nil {
		return fmt.Errorf("deleting otp: %w", err)
	}
	return nil
}

func (c *Cache) key(recipient string) string {
	sum := sha256.Sum256([]byte(recipient))
	return c.prefix + ":" + hex.EncodeToString(sum[:])
}
