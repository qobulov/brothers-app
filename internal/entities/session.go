package entities

import (
	"time"

	"github.com/google/uuid"
)

// UserSession stores the one active refresh session allowed per user.
type UserSession struct {
	ID               uuid.UUID  `json:"id"`
	UserID           uuid.UUID  `json:"user_id"`
	RefreshTokenHash string     `json:"-"`
	DeviceID         string     `json:"device_id,omitempty"`
	DeviceName       string     `json:"device_name,omitempty"`
	ExpiresAt        time.Time  `json:"expires_at"`
	CreatedAt        time.Time  `json:"created_at"`
	RevokedAt        *time.Time `json:"-"`
}
