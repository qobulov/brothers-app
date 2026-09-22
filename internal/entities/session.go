package entities

import (
	"time"

	"github.com/google/uuid"
)

// UserSession stores the one active refresh session allowed per user.
type UserSession struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	UserID           uuid.UUID  `gorm:"type:uuid;index;not null" json:"user_id"`
	RefreshTokenHash string     `gorm:"index;not null" json:"-"`
	DeviceID         string     `json:"device_id,omitempty"`
	DeviceName       string     `json:"device_name,omitempty"`
	ExpiresAt        time.Time  `gorm:"not null" json:"expires_at"`
	CreatedAt        time.Time  `json:"created_at"`
	RevokedAt        *time.Time `json:"-"`
}

func (s *UserSession) BeforeCreate() error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}
