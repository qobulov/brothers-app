package service

import (
	"context"
	"crypto/hmac"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/internal/auth/dto"
	"github.com/qobulov/brothers-app/internal/entities"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/config"
	"github.com/qobulov/brothers-app/pkg/helpers"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	registrationPurpose  = "registration"
	passwordResetPurpose = "password_reset"
	phoneChangePurpose   = "phone_change"
	awaitingBotStart     = "awaiting_bot_start"
	otpSent              = "otp_sent"
	deliveryPending      = "delivery_pending"
	deliveryFailed       = "delivery_failed"
)

// OTPSender is the delivery port used by auth; Telegram is one adapter.
type OTPSender interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
}

type Service struct {
	db        *gorm.DB
	cfg       *config.Config
	otpSender OTPSender
	now       func() time.Time
}

// otpRecord is a temporary in-service persistence shape. OTP storage is not
// part of the initial database schema while the auth server is being finalized.
type otpRecord struct {
	ID                  uuid.UUID  `gorm:"type:uuid;primaryKey"`
	UserID              *uuid.UUID `gorm:"type:uuid;index"`
	Phone               string     `gorm:"index;not null"`
	Purpose             string     `gorm:"index;not null"`
	StartTokenHash      string     `gorm:"column:start_token_hash;index;not null"`
	DeliveryChatID      *int64     `gorm:"column:delivery_chat_id"`
	StartedAt           *time.Time `gorm:"column:started_at"`
	OTPHash             string     `gorm:"column:otp_hash"`
	DeliveryStatus      string
	ExpiresAt           time.Time `gorm:"index;not null"`
	AttemptCount        int       `gorm:"not null;default:0"`
	ResendCount         int       `gorm:"not null;default:0"`
	LastSentAt          *time.Time
	ConsumedAt          *time.Time
	SupersededAt        *time.Time
	ResetTokenHash      string
	ResetTokenExpiresAt *time.Time
	ResetUsedAt         *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func New(db *gorm.DB, cfg *config.Config, otpSender OTPSender) *Service {
	return &Service{db: db, cfg: cfg, otpSender: otpSender, now: time.Now}
}

func (s *Service) Register(ctx context.Context, req dto.RegisterRequest) (dto.StartData, error) {
	phone, err := helpers.NormalizePhone(req.Phone)
	if err != nil || req.Password == "" || req.Username == "" || req.FirstName == "" {
		return dto.StartData{}, apperror.ErrInvalidData
	}
	if len(req.Password) < 8 || len(req.Username) > 50 {
		return dto.StartData{}, apperror.ErrInvalidData
	}
	if req.Language == "" {
		req.Language = "uz"
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return dto.StartData{}, fmt.Errorf("hashing password: %w", err)
	}

	startToken, err := helpers.RandomToken(32)
	if err != nil {
		return dto.StartData{}, fmt.Errorf("creating telegram start token: %w", err)
	}
	now := s.now().UTC()
	expires := now.Add(time.Duration(s.cfg.OTPExpiration) * time.Second)
	var user entities.User
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Where("phone = ? OR username = ?", phone, req.Username).First(&user)
		if query.Error == nil && user.IsActive {
			return apperror.ErrAlreadyExists
		}
		if query.Error != nil && !errors.Is(query.Error, gorm.ErrRecordNotFound) {
			return fmt.Errorf("finding registration user: %w", query.Error)
		}
		if errors.Is(query.Error, gorm.ErrRecordNotFound) {
			user = entities.User{ID: uuid.New()}
		}
		user.Phone = phone
		user.Username = req.Username
		// The legacy schema still has a non-null unique email column. Until the
		// versioned migration removes it, keep a deterministic private placeholder
		// so multiple phone-based accounts can coexist.
		user.Email = phone + "@telegram.invalid"
		user.FirstName = req.FirstName
		user.LastName = req.LastName
		user.Name = strings.TrimSpace(req.FirstName + " " + req.LastName)
		user.PasswordHash = string(hash)
		user.Language = req.Language
		user.AvatarURL = req.AvatarURL
		user.IsActive = false
		if err := tx.Save(&user).Error; err != nil {
			return fmt.Errorf("saving pending user: %w", err)
		}
		if err := tx.Model(&otpRecord{}).
			Where("purpose = ? AND phone = ? AND consumed_at IS NULL AND superseded_at IS NULL", registrationPurpose, phone).
			Updates(map[string]interface{}{"superseded_at": now, "updated_at": now}).Error; err != nil {
			return fmt.Errorf("superseding registration challenge: %w", err)
		}
		userID := user.ID
		challenge := &otpRecord{
			ID: uuid.New(), UserID: &userID, Phone: phone, Purpose: registrationPurpose,
			StartTokenHash: helpers.HashSecret(startToken), DeliveryStatus: awaitingBotStart,
			ExpiresAt: expires, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(challenge).Error; err != nil {
			return fmt.Errorf("creating registration challenge: %w", err)
		}
		return nil
	})
	if err != nil {
		return dto.StartData{}, err
	}
	return s.startData(startToken, expires), nil
}

func (s *Service) VerifyRegistration(ctx context.Context, phone, otp string) (dto.AuthData, error) {
	phone, err := helpers.NormalizePhone(phone)
	if err != nil || !helpers.ValidOTP(otp) {
		return dto.AuthData{}, apperror.ErrInvalidOTP
	}
	var result dto.AuthData
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var challenge otpRecord
		if err := tx.Where("purpose = ? AND phone = ? AND consumed_at IS NULL AND superseded_at IS NULL AND expires_at > ?", registrationPurpose, phone, s.now().UTC()).First(&challenge).Error; err != nil {
			return apperror.ErrInvalidOTP
		}
		if challenge.AttemptCount >= s.cfg.OTPMaxAttempts {
			return apperror.ErrLimitExceeded
		}
		if !hmac.Equal([]byte(challenge.OTPHash), []byte(s.hashOTP(otp))) || challenge.DeliveryStatus != otpSent || challenge.DeliveryChatID == nil {
			_ = tx.Model(&challenge).UpdateColumn("attempt_count", gorm.Expr("attempt_count + 1")).Error
			return apperror.ErrInvalidOTP
		}
		now := s.now().UTC()
		consumeResult := tx.Model(&challenge).Where("consumed_at IS NULL").Updates(map[string]interface{}{"consumed_at": now, "updated_at": now})
		if consumeResult.Error != nil {
			return fmt.Errorf("consuming registration otp: %w", consumeResult.Error)
		}
		if consumeResult.RowsAffected != 1 {
			return apperror.ErrInvalidOTP
		}
		if challenge.UserID == nil {
			return apperror.ErrInvalidOTP
		}
		var user entities.User
		if err := tx.First(&user, "id = ?", *challenge.UserID).Error; err != nil {
			return fmt.Errorf("loading registration user: %w", err)
		}
		user.IsActive = true
		if err := tx.Save(&user).Error; err != nil {
			return fmt.Errorf("activating user: %w", err)
		}
		pair, err := s.issueSession(tx, user)
		if err != nil {
			return err
		}
		result = dto.AuthData{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken, User: SafeUser(user)}
		return nil
	})
	return result, err
}

func (s *Service) Login(ctx context.Context, identifier, password string) (dto.AuthData, error) {
	identifier = strings.TrimSpace(identifier)
	phoneIdentifier := identifier
	if normalized, err := helpers.NormalizePhone(identifier); err == nil {
		phoneIdentifier = normalized
	}
	var result dto.AuthData
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user entities.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("username = ? OR phone = ? OR email = ?", identifier, phoneIdentifier, strings.ToLower(identifier)).First(&user).Error; err != nil {
			return apperror.ErrInvalidCredentials
		}
		passwordHash := user.PasswordHash
		if passwordHash == "" {
			passwordHash = user.Password
		}
		if !user.IsActive || bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
			return apperror.ErrInvalidCredentials
		}
		now := s.now().UTC()
		if err := tx.Model(&user).Updates(map[string]interface{}{"last_login_at": now, "updated_at": now}).Error; err != nil {
			return fmt.Errorf("updating login time: %w", err)
		}
		user.LastLoginAt = &now
		user.UpdatedAt = now
		pair, err := s.issueSession(tx, user)
		if err != nil {
			return err
		}
		result = dto.AuthData{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken, User: SafeUser(user)}
		return nil
	})
	return result, err
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (dto.AuthData, error) {
	if refreshToken == "" {
		return dto.AuthData{}, apperror.ErrUnauthorized
	}
	var result dto.AuthData
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session entities.UserSession
		if err := tx.Where("refresh_token_hash = ? AND revoked_at IS NULL AND expires_at > ?", helpers.HashSecret(refreshToken), s.now().UTC()).First(&session).Error; err != nil {
			return apperror.ErrUnauthorized
		}
		var user entities.User
		if err := tx.First(&user, "id = ? AND is_active = ?", session.UserID, true).Error; err != nil {
			return apperror.ErrUnauthorized
		}
		newRefresh, err := helpers.RandomToken(32)
		if err != nil {
			return fmt.Errorf("creating refresh token: %w", err)
		}
		now := s.now().UTC()
		resultCode := tx.Model(&entities.UserSession{}).
			Where("id = ? AND refresh_token_hash = ? AND revoked_at IS NULL AND expires_at > ?", session.ID, helpers.HashSecret(refreshToken), now).
			Updates(map[string]interface{}{"refresh_token_hash": helpers.HashSecret(newRefresh), "expires_at": now.Add(30 * 24 * time.Hour)})
		if resultCode.Error != nil {
			return fmt.Errorf("rotating refresh token: %w", resultCode.Error)
		}
		if resultCode.RowsAffected != 1 {
			return apperror.ErrUnauthorized
		}
		accessToken, err := s.signAccessToken(user, session.ID, now)
		if err != nil {
			return err
		}
		result = dto.AuthData{AccessToken: accessToken, RefreshToken: newRefresh, User: SafeUser(user)}
		return nil
	})
	return result, err
}

func (s *Service) CurrentUser(ctx context.Context, userID uuid.UUID) (map[string]interface{}, error) {
	var user entities.User
	if err := s.db.WithContext(ctx).First(&user, "id = ? AND is_active = ?", userID, true).Error; err != nil {
		return nil, apperror.ErrUnauthorized
	}
	return SafeUser(user), nil
}

func (s *Service) ForgotPassword(ctx context.Context, phone string) (dto.StartData, error) {
	phone, err := helpers.NormalizePhone(phone)
	if err != nil {
		return dto.StartData{}, apperror.ErrInvalidData
	}
	var user entities.User
	queryErr := s.db.WithContext(ctx).Where("phone = ?", phone).First(&user).Error
	var userID *uuid.UUID
	if queryErr == nil {
		userID = &user.ID
	}
	return s.createChallenge(ctx, passwordResetPurpose, phone, userID)
}

func (s *Service) VerifyPasswordOTP(ctx context.Context, phone, otp string) (dto.ResetVerifyData, error) {
	phone, err := helpers.NormalizePhone(phone)
	if err != nil || !helpers.ValidOTP(otp) {
		return dto.ResetVerifyData{}, apperror.ErrInvalidOTP
	}
	var result dto.ResetVerifyData
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var challenge otpRecord
		if err := tx.Where("purpose = ? AND phone = ? AND consumed_at IS NULL AND superseded_at IS NULL AND expires_at > ?", passwordResetPurpose, phone, s.now().UTC()).First(&challenge).Error; err != nil {
			return apperror.ErrInvalidOTP
		}
		if challenge.AttemptCount >= s.cfg.OTPMaxAttempts {
			return apperror.ErrLimitExceeded
		}
		if challenge.DeliveryStatus != otpSent || !hmac.Equal([]byte(challenge.OTPHash), []byte(s.hashOTP(otp))) || challenge.UserID == nil {
			_ = tx.Model(&challenge).UpdateColumn("attempt_count", gorm.Expr("attempt_count + 1")).Error
			return apperror.ErrInvalidOTP
		}
		token, err := helpers.RandomToken(32)
		if err != nil {
			return fmt.Errorf("creating reset token: %w", err)
		}
		now := s.now().UTC()
		expires := now.Add(10 * time.Minute)
		consumeResult := tx.Model(&challenge).Where("consumed_at IS NULL").Updates(map[string]interface{}{"consumed_at": now, "reset_token_hash": helpers.HashSecret(token), "reset_token_expires_at": expires, "updated_at": now})
		if consumeResult.Error != nil {
			return fmt.Errorf("storing reset token: %w", consumeResult.Error)
		}
		if consumeResult.RowsAffected != 1 {
			return apperror.ErrInvalidOTP
		}
		result = dto.ResetVerifyData{ResetToken: token, ExpiresAt: expires.Format(time.RFC3339)}
		return nil
	})
	return result, err
}

func (s *Service) ResetPassword(ctx context.Context, req dto.ResetPasswordRequest) error {
	if req.Password == "" || len(req.Password) < 8 || req.Password != req.ConfirmPassword {
		return apperror.ErrInvalidData
	}
	var challenge otpRecord
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hashing reset password: %w", err)
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("reset_token_hash = ? AND reset_used_at IS NULL AND reset_token_expires_at > ?", helpers.HashSecret(req.ResetToken), s.now().UTC()).First(&challenge).Error; err != nil {
			return apperror.ErrInvalidResetToken
		}
		if challenge.UserID == nil {
			return apperror.ErrInvalidResetToken
		}
		if err := tx.Model(&entities.User{}).Where("id = ?", *challenge.UserID).Update("password_hash", string(hash)).Error; err != nil {
			return fmt.Errorf("updating password: %w", err)
		}
		now := s.now().UTC()
		result := tx.Model(&otpRecord{}).Where("id = ? AND reset_used_at IS NULL", challenge.ID).Update("reset_used_at", now)
		if result.Error != nil {
			return fmt.Errorf("consuming reset token: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return apperror.ErrInvalidResetToken
		}
		return tx.Model(&entities.UserSession{}).Where("user_id = ? AND revoked_at IS NULL", *challenge.UserID).Update("revoked_at", now).Error
	})
}

func (s *Service) Logout(ctx context.Context, userID, sessionID uuid.UUID) error {
	now := s.now().UTC()
	result := s.db.WithContext(ctx).Model(&entities.UserSession{}).Where("id = ? AND user_id = ? AND revoked_at IS NULL", sessionID, userID).Update("revoked_at", now)
	if result.Error != nil {
		return fmt.Errorf("revoking session: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperror.ErrSessionRevoked
	}
	return nil
}

func (s *Service) PhoneChangeRequest(ctx context.Context, userID uuid.UUID, newPhone string) (dto.StartData, error) {
	phone, err := helpers.NormalizePhone(newPhone)
	if err != nil {
		return dto.StartData{}, apperror.ErrInvalidData
	}
	var existing entities.User
	if err := s.db.WithContext(ctx).Where("phone = ? AND id <> ?", phone, userID).First(&existing).Error; err == nil {
		return dto.StartData{}, apperror.ErrAlreadyExists
	}
	return s.createChallengeForUser(ctx, phoneChangePurpose, phone, &userID)
}

func (s *Service) PhoneChangeConfirm(ctx context.Context, userID uuid.UUID, newPhone, otp string) error {
	phone, err := helpers.NormalizePhone(newPhone)
	if err != nil || !helpers.ValidOTP(otp) {
		return apperror.ErrInvalidOTP
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var challenge otpRecord
		if err := tx.Where("purpose = ? AND user_id = ? AND phone = ? AND consumed_at IS NULL AND superseded_at IS NULL AND expires_at > ?", phoneChangePurpose, userID, phone, s.now().UTC()).First(&challenge).Error; err != nil {
			return apperror.ErrInvalidOTP
		}
		if challenge.AttemptCount >= s.cfg.OTPMaxAttempts {
			return apperror.ErrLimitExceeded
		}
		if challenge.DeliveryStatus != otpSent || !hmac.Equal([]byte(challenge.OTPHash), []byte(s.hashOTP(otp))) {
			_ = tx.Model(&challenge).UpdateColumn("attempt_count", gorm.Expr("attempt_count + 1")).Error
			return apperror.ErrInvalidOTP
		}
		now := s.now().UTC()
		result := tx.Model(&challenge).Where("consumed_at IS NULL").Update("consumed_at", now)
		if result.Error != nil {
			return fmt.Errorf("consuming phone change otp: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return apperror.ErrInvalidOTP
		}
		return tx.Model(&entities.User{}).Where("id = ?", userID).Updates(map[string]interface{}{"phone": phone, "updated_at": now}).Error
	})
}

func (s *Service) HandleBotStart(ctx context.Context, startToken string, chatID int64) error {
	var challenge otpRecord
	var otp string
	shouldSend := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("start_token_hash = ? AND consumed_at IS NULL AND superseded_at IS NULL AND expires_at > ?", helpers.HashSecret(startToken), s.now().UTC()).First(&challenge).Error; err != nil {
			return apperror.ErrInvalidOTP
		}
		if challenge.StartedAt != nil && challenge.DeliveryStatus == otpSent {
			return nil
		}
		var err error
		otp, err = helpers.GenerateOTP()
		if err != nil {
			return fmt.Errorf("generating otp: %w", err)
		}
		now := s.now().UTC()
		result := tx.Model(&challenge).Updates(map[string]interface{}{"delivery_chat_id": chatID, "started_at": now, "otp_hash": s.hashOTP(otp), "delivery_status": deliveryPending, "last_sent_at": now, "updated_at": now})
		if result.Error != nil {
			return fmt.Errorf("binding otp delivery chat: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return apperror.ErrInvalidOTP
		}
		shouldSend = true
		return nil
	})
	if err != nil {
		return err
	}
	if !shouldSend {
		return nil
	}
	if s.otpSender == nil {
		_ = s.db.WithContext(ctx).Model(&challenge).Updates(map[string]interface{}{"delivery_status": deliveryFailed, "updated_at": s.now().UTC()}).Error
		return apperror.ErrTelegramUnavailable
	}
	if err := s.otpSender.SendMessage(ctx, chatID, "Ваш код подтверждения: "+otp); err != nil {
		_ = s.db.WithContext(ctx).Model(&challenge).Updates(map[string]interface{}{"delivery_status": deliveryFailed, "updated_at": s.now().UTC()}).Error
		return fmt.Errorf("sending telegram otp: %w", err)
	}
	if err := s.db.WithContext(ctx).Model(&challenge).Update("delivery_status", otpSent).Error; err != nil {
		return fmt.Errorf("marking telegram otp delivered: %w", err)
	}
	return nil
}

func (s *Service) Resend(ctx context.Context, purpose, phone string, userID *uuid.UUID) error {
	phone, err := helpers.NormalizePhone(phone)
	if err != nil {
		return apperror.ErrInvalidData
	}
	var challenge otpRecord
	otp, err := helpers.GenerateOTP()
	if err != nil {
		return fmt.Errorf("generating otp: %w", err)
	}
	now := s.now().UTC()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("purpose = ? AND phone = ? AND consumed_at IS NULL AND superseded_at IS NULL", purpose, phone)
		if userID != nil {
			query = query.Where("user_id = ?", *userID)
		}
		if err := query.First(&challenge).Error; err != nil {
			return apperror.ErrBotNotStarted
		}
		if challenge.StartedAt == nil || challenge.DeliveryChatID == nil {
			return apperror.ErrBotNotStarted
		}
		if challenge.LastSentAt != nil && now.Before(challenge.LastSentAt.Add(time.Duration(s.cfg.OTPResendCooldown)*time.Second)) {
			return apperror.ErrLimitExceeded
		}
		result := tx.Model(&challenge).Updates(map[string]interface{}{"otp_hash": s.hashOTP(otp), "delivery_status": deliveryPending, "last_sent_at": now, "resend_count": gorm.Expr("resend_count + 1"), "attempt_count": 0, "updated_at": now})
		if result.Error != nil {
			return fmt.Errorf("updating otp: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return apperror.ErrBotNotStarted
		}
		return nil
	})
	if err != nil {
		return err
	}
	if s.otpSender == nil {
		_ = s.db.WithContext(ctx).Model(&challenge).Update("delivery_status", deliveryFailed).Error
		return apperror.ErrTelegramUnavailable
	}
	if err := s.otpSender.SendMessage(ctx, *challenge.DeliveryChatID, "Ваш новый код подтверждения: "+otp); err != nil {
		_ = s.db.WithContext(ctx).Model(&challenge).Update("delivery_status", deliveryFailed).Error
		return fmt.Errorf("sending telegram otp: %w", err)
	}
	if err := s.db.WithContext(ctx).Model(&challenge).Update("delivery_status", otpSent).Error; err != nil {
		return fmt.Errorf("marking resent telegram otp delivered: %w", err)
	}
	return nil
}

type tokenPair struct {
	AccessToken  string
	RefreshToken string
}

func (s *Service) issueSession(tx *gorm.DB, user entities.User) (tokenPair, error) {
	now := s.now().UTC()
	refresh, err := helpers.RandomToken(32)
	if err != nil {
		return tokenPair{}, fmt.Errorf("creating refresh token: %w", err)
	}
	session := entities.UserSession{ID: uuid.New(), UserID: user.ID, RefreshTokenHash: helpers.HashSecret(refresh), ExpiresAt: now.Add(30 * 24 * time.Hour), CreatedAt: now}
	if err := tx.Model(&entities.UserSession{}).Where("user_id = ? AND revoked_at IS NULL", user.ID).Update("revoked_at", now).Error; err != nil {
		return tokenPair{}, fmt.Errorf("revoking previous session: %w", err)
	}
	if err := tx.Create(&session).Error; err != nil {
		return tokenPair{}, fmt.Errorf("creating session: %w", err)
	}
	token, err := s.signAccessToken(user, session.ID, now)
	if err != nil {
		return tokenPair{}, err
	}
	return tokenPair{AccessToken: token, RefreshToken: refresh}, nil
}

func (s *Service) signAccessToken(user entities.User, sessionID uuid.UUID, now time.Time) (string, error) {
	claims := jwt.MapClaims{"sub": user.ID.String(), "sid": sessionID.String(), "type": "access", "iss": s.cfg.JWTIssuer, "aud": s.cfg.JWTAudience, "iat": now.Unix(), "exp": now.Add(time.Duration(s.cfg.JWTExpiration) * time.Second).Unix()}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return "", fmt.Errorf("signing access token: %w", err)
	}
	return token, nil
}

func (s *Service) createChallenge(ctx context.Context, purpose, phone string, userID *uuid.UUID) (dto.StartData, error) {
	return s.createChallengeForUser(ctx, purpose, phone, userID)
}

func (s *Service) createChallengeForUser(ctx context.Context, purpose, phone string, userID *uuid.UUID) (dto.StartData, error) {
	token, err := helpers.RandomToken(32)
	if err != nil {
		return dto.StartData{}, fmt.Errorf("creating telegram start token: %w", err)
	}
	now := s.now().UTC()
	expires := now.Add(time.Duration(s.cfg.OTPExpiration) * time.Second)
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&otpRecord{}).Where("purpose = ? AND phone = ? AND consumed_at IS NULL AND superseded_at IS NULL", purpose, phone)
		if userID != nil {
			query = query.Where("user_id = ?", *userID)
		}
		if err := query.Updates(map[string]interface{}{"superseded_at": now, "updated_at": now}).Error; err != nil {
			return fmt.Errorf("superseding auth challenge: %w", err)
		}
		challenge := &otpRecord{ID: uuid.New(), UserID: userID, Phone: phone, Purpose: purpose, StartTokenHash: helpers.HashSecret(token), DeliveryStatus: awaitingBotStart, ExpiresAt: expires, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(challenge).Error; err != nil {
			return fmt.Errorf("creating auth challenge: %w", err)
		}
		return nil
	})
	if err != nil {
		return dto.StartData{}, err
	}
	return s.startData(token, expires), nil
}

func (s *Service) startData(token string, expires time.Time) dto.StartData {
	link := ""
	if s.cfg.TelegramBotUsername != "" {
		link = "https://t.me/" + strings.TrimPrefix(s.cfg.TelegramBotUsername, "@") + "?start=" + token
	}
	return dto.StartData{TelegramDeepLink: link, ExpiresAt: expires.Format(time.RFC3339), ResendIn: s.cfg.OTPResendCooldown}
}

func (s *Service) hashOTP(otp string) string {
	return helpers.HashHMAC(otp, s.cfg.OTPPepper)
}

func SafeUser(user entities.User) map[string]interface{} {
	return map[string]interface{}{"id": user.ID, "phone": user.Phone, "username": user.Username, "first_name": user.FirstName, "last_name": user.LastName, "avatar_url": user.AvatarURL, "language": user.Language, "is_active": user.IsActive, "created_at": user.CreatedAt, "updated_at": user.UpdatedAt}
}

func ParseUUID(value string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, apperror.ErrUnauthorized
	}
	return parsed, nil
}

func ParseRefreshExpiry(value string) (time.Time, error) {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(seconds, 0), nil
}
