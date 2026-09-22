package entities

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	Email        string         `gorm:"uniqueIndex" json:"email,omitempty"`
	Password     string         `json:"-" gorm:"column:password"`
	PasswordHash string         `json:"-" gorm:"column:password_hash"`
	Name         string         `json:"name,omitempty"`
	Phone        string         `json:"phone,omitempty"`
	Username     string         `json:"username,omitempty"`
	FirstName    string         `json:"first_name,omitempty"`
	LastName     string         `json:"last_name,omitempty"`
	AvatarURL    string         `json:"avatar_url,omitempty"`
	Language     string         `json:"language,omitempty"`
	IsActive     bool           `json:"is_active" gorm:"default:false"`
	LastLoginAt  *time.Time     `json:"last_login_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"index"`
}

func (u *User) BeforeCreate(tx *gorm.DB) (err error) {
	u.ID = uuid.New()
	return
}
