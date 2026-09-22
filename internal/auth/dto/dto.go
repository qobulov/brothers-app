package dto

type RegisterRequest struct {
	Phone     string `json:"phone"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Password  string `json:"password"`
	Language  string `json:"language"`
	AvatarURL string `json:"avatar_url"`
}

type LoginRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

type OTPRequest struct {
	Phone string `json:"phone"`
	OTP   string `json:"otp"`
}

type ForgotPasswordRequest struct {
	Phone string `json:"phone"`
}

type ResetPasswordRequest struct {
	ResetToken      string `json:"reset_token"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirm_password"`
}

type PhoneChangeRequest struct {
	NewPhone string `json:"new_phone"`
}

type PhoneChangeConfirmRequest struct {
	NewPhone string `json:"new_phone"`
	OTP      string `json:"otp"`
}

type StartData struct {
	TelegramDeepLink string `json:"telegram_deep_link"`
	ExpiresAt        string `json:"expires_at"`
	ResendIn         int    `json:"resend_in"`
}

type AuthData struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	User         interface{} `json:"user"`
}

type ResetVerifyData struct {
	ResetToken string `json:"reset_token"`
	ExpiresAt  string `json:"expires_at"`
}
