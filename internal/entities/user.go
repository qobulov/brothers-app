package entities

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID  `json:"id"`
	Password     string     `json:"-"`
	PasswordHash string     `json:"-"`
	Name         string     `json:"name,omitempty"`
	Email        string     `json:"email,omitempty"`
	Phone        string     `json:"phone,omitempty"`
	Username     string     `json:"username,omitempty"`
	FirstName    string     `json:"first_name,omitempty"`
	LastName     string     `json:"last_name,omitempty"`
	AvatarURL    string     `json:"avatar_url,omitempty"`
	Language     string     `json:"language,omitempty"`
	IsActive     bool       `json:"is_active"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
	CreatedAt    time.Time  `json:"-"`
	UpdatedAt    time.Time  `json:"-"`
	DeletedAt    *time.Time `json:"-"`
}
