package authdto

import (
	"time"

	"github.com/google/uuid"
)

type RegisterRequest struct {
	Phone     string `json:"phone" example:"+998901234567"`
	Username  string `json:"username" example:"qobulov"`
	FirstName string `json:"first_name" example:"Qobul"`
	LastName  string `json:"last_name" example:"Qobulov"`
	Password  string `json:"password" example:"strong-password"`
	Language  string `json:"language" example:"uz"`
	AvatarURL string `json:"avatar_url" example:"https://example.com/avatar.jpg"`
}

// LoginRequest contains username-or-phone credentials.
type LoginRequest struct {
	Login    string `json:"login" example:"qobulov"`
	Password string `json:"password" example:"strong-password"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" example:"opaque-refresh-token"`
}

type PhoneRequest struct {
	Phone string `json:"phone" example:"+998901234567"`
}

type OTPVerifyRequest struct {
	Phone string `json:"phone" example:"+998901234567"`
	OTP   string `json:"otp" example:"123456"`
}

type ForgotPasswordRequest struct {
	Phone string `json:"phone" example:"+998901234567"`
}

type ResetPasswordRequest struct {
	ResetToken      string `json:"reset_token" example:"opaque-reset-token"`
	Password        string `json:"password" example:"new-strong-password"`
	ConfirmPassword string `json:"confirm_password" example:"new-strong-password"`
}

type PhoneChangeRequest struct {
	NewPhone string `json:"new_phone" example:"+998901234568"`
}

type PhoneChangeConfirmRequest struct {
	NewPhone string `json:"new_phone" example:"+998901234568"`
	OTP      string `json:"otp" example:"123456"`
}

// UpdateProfileRequest contains only user-editable profile fields. Pointer
// fields preserve PATCH semantics: omitted fields stay unchanged.
type UpdateProfileRequest struct {
	FirstName *string `json:"first_name,omitempty" example:"Qobul"`
	LastName  *string `json:"last_name,omitempty" example:"Qobulov"`
	AvatarURL *string `json:"avatar_url,omitempty" example:"https://example.com/avatar.jpg"`
	Language  *string `json:"language,omitempty" enums:"uz,ru,en" example:"uz"`
}

type StartData struct {
	TelegramDeepLink string `json:"telegram_deep_link"`
	ExpiresAt        string `json:"expires_at"`
	ResendIn         int    `json:"resend_in"`
}

// AuthData contains the issued session tokens and authenticated user.
type AuthData struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	User         UserData `json:"user"`
}

type ResetVerifyData struct {
	ResetToken string `json:"reset_token"`
	ExpiresAt  string `json:"expires_at"`
}

// UserData is the safe profile representation returned by auth endpoints.
type UserData struct {
	ID          uuid.UUID  `json:"id"`
	Phone       string     `json:"phone"`
	Username    string     `json:"username"`
	FirstName   string     `json:"first_name"`
	LastName    string     `json:"last_name"`
	AvatarURL   string     `json:"avatar_url"`
	Language    string     `json:"language"`
	IsActive    bool       `json:"is_active"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
