package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	dto "github.com/qobulov/brothers-app/internal/auth/dto"
	"github.com/qobulov/brothers-app/internal/auth/otp"
	db "github.com/qobulov/brothers-app/internal/db"
	"github.com/qobulov/brothers-app/internal/entities"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/config"
	"github.com/qobulov/brothers-app/pkg/helpers"
	"golang.org/x/crypto/bcrypt"
)

const (
	registrationPurpose  = "registration"
	passwordResetPurpose = "password_reset"
	phoneChangePurpose   = "phone_change"
)

type OTPSender interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
}

type Service struct {
	pool      *pgxpool.Pool
	queries   *db.Queries
	otp       *otp.Cache
	cfg       *config.Config
	otpSender OTPSender
	now       func() time.Time
}

func New(pool *pgxpool.Pool, otpCache *otp.Cache, cfg *config.Config, sender OTPSender) *Service {
	return &Service{pool: pool, queries: db.New(pool), otp: otpCache, cfg: cfg, otpSender: sender, now: time.Now}
}

func (s *Service) SendOTP(ctx context.Context, req dto.SendOTPRequest) (dto.StartData, error) {
	phone, err := helpers.NormalizePhone(req.Phone)
	if err != nil {
		return dto.StartData{}, apperror.ErrInvalidData
	}
	purpose := strings.ToLower(strings.TrimSpace(req.Purpose))
	if purpose == "" {
		purpose = registrationPurpose
	}
	if purpose != registrationPurpose {
		return dto.StartData{}, apperror.ErrInvalidData
	}

	_, err = s.queries.GetUserByPhone(ctx, db.GetUserByPhoneParams{Phone: text(phone)})
	if err == nil {
		return dto.StartData{}, apperror.ErrRegistrationIdentityExists
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return dto.StartData{}, fmt.Errorf("checking registration phone: %w", err)
	}
	// A configured fixed OTP is stored directly instead of reusing a Telegram
	// chat binding that may belong to another bot.
	if s.configuredOTP() != "" {
		return s.createOTPFlow(ctx, purpose, phone, uuid.Nil)
	}

	if _, err := s.otp.Get(ctx, s.chatKey(purpose, phone)); err == nil {
		if err := s.Resend(ctx, purpose, phone, nil); err != nil {
			return dto.StartData{}, err
		}
		expires := s.now().UTC().Add(time.Duration(s.cfg.OTPExpiration) * time.Second)
		return s.startData("", expires), nil
	} else if !errors.Is(err, otp.ErrNotFound) {
		return dto.StartData{}, err
	}

	return s.createOTPFlow(ctx, purpose, phone, uuid.Nil)
}

func (s *Service) Register(ctx context.Context, req dto.RegisterRequest) (dto.RegisterData, error) {
	phone, err := helpers.NormalizePhone(req.Phone)
	username := strings.TrimSpace(req.Username)
	firstName := strings.TrimSpace(req.FirstName)
	lastName := strings.TrimSpace(req.LastName)
	if err != nil || req.Password == "" || username == "" || firstName == "" || len(req.Password) < 8 || len(username) > 50 {
		return dto.RegisterData{}, apperror.ErrInvalidData
	}
	if !helpers.ValidOTP(req.OTPCode) {
		return dto.RegisterData{}, apperror.ErrInvalidOTP
	}
	if req.Language == "" {
		req.Language = "uz"
	}
	req.Language = strings.ToLower(strings.TrimSpace(req.Language))
	if req.Language != "uz" && req.Language != "ru" && req.Language != "en" {
		return dto.RegisterData{}, apperror.ErrInvalidData
	}
	if req.AvatarURL != "" {
		if _, err := optionalAvatarURL(&req.AvatarURL); err != nil {
			return dto.RegisterData{}, err
		}
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return dto.RegisterData{}, fmt.Errorf("hashing password: %w", err)
	}

	var result dto.RegisterData
	err = s.withTx(ctx, func(q *db.Queries) error {
		_, findErr := q.GetUserByPhoneOrUsername(ctx, db.GetUserByPhoneOrUsernameParams{Phone: text(phone), Username: text(username)})
		if findErr == nil {
			return apperror.ErrRegistrationIdentityExists
		}
		if findErr != nil && !errors.Is(findErr, pgx.ErrNoRows) {
			return fmt.Errorf("checking registration identity: %w", findErr)
		}
		if err := s.consumeOTP(ctx, registrationPurpose, phone, req.OTPCode); err != nil {
			return err
		}

		now := timestamp(s.now().UTC())
		user, findErr := q.CreateAuthUser(ctx, db.CreateAuthUserParams{
			ID: pgUUID(uuid.New()), PasswordHash: text(string(passwordHash)),
			Name: text(strings.TrimSpace(firstName + " " + lastName)), Phone: text(phone), Username: text(username),
			FirstName: text(firstName), LastName: text(lastName), AvatarUrl: text(req.AvatarURL), Language: req.Language, CreatedAt: now,
		})
		if findErr != nil {
			return fmt.Errorf("saving registration user: %w", findErr)
		}
		pair, issueErr := s.issueSession(ctx, q, user)
		if issueErr != nil {
			return issueErr
		}
		result = dto.RegisterData{
			Tokens: tokenData(pair),
			User: dto.RegisterUserData{
				ID: uuidFromPG(user.ID), FullName: user.Name.String, Phone: user.Phone.String, Role: "user",
			},
		}
		return nil
	})
	if isUniqueViolation(err) {
		return dto.RegisterData{}, apperror.ErrRegistrationIdentityExists
	}
	return result, err
}

func (s *Service) Login(ctx context.Context, login, password string) (dto.AuthData, error) {
	login, phone := loginIdentifiers(login)
	if login == "" || password == "" {
		return dto.AuthData{}, apperror.ErrInvalidCredentials
	}
	var result dto.AuthData
	err := s.withTx(ctx, func(q *db.Queries) error {
		user, queryErr := q.GetUserByLogin(ctx, db.GetUserByLoginParams{Username: text(login), Phone: text(phone)})
		if queryErr != nil {
			if errors.Is(queryErr, pgx.ErrNoRows) {
				return apperror.ErrInvalidCredentials
			}
			return fmt.Errorf("finding login user: %w", queryErr)
		}
		hash := user.PasswordHash.String
		if hash == "" {
			hash = user.Password.String
		}
		if !user.IsActive || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
			return apperror.ErrInvalidCredentials
		}
		user, queryErr = q.UpdateUserLogin(ctx, db.UpdateUserLoginParams{ID: user.ID, LastLoginAt: timestamp(s.now().UTC())})
		if queryErr != nil {
			return fmt.Errorf("updating login time: %w", queryErr)
		}
		pair, issueErr := s.issueSession(ctx, q, user)
		if issueErr != nil {
			return issueErr
		}
		result = dto.AuthData{
			Tokens: tokenData(pair),
			User:   SafeUser(toEntity(user)),
		}
		return nil
	})
	return result, err
}

func loginIdentifiers(value string) (username, phone string) {
	username = strings.TrimSpace(value)
	if normalized, err := helpers.NormalizePhone(username); err == nil {
		phone = normalized
	}
	return username, phone
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (dto.AuthData, error) {
	if refreshToken == "" {
		return dto.AuthData{}, apperror.ErrUnauthorized
	}
	var result dto.AuthData
	err := s.withTx(ctx, func(q *db.Queries) error {
		now := s.now().UTC()
		session, queryErr := q.GetSessionByRefreshHash(ctx, db.GetSessionByRefreshHashParams{RefreshTokenHash: helpers.HashSecret(refreshToken), ExpiresAt: timestamp(now)})
		if queryErr != nil {
			return apperror.ErrUnauthorized
		}
		user, queryErr := q.GetActiveUser(ctx, db.GetActiveUserParams{ID: session.UserID})
		if queryErr != nil {
			return apperror.ErrUnauthorized
		}
		newRefresh, tokenErr := helpers.RandomToken(32)
		if tokenErr != nil {
			return fmt.Errorf("creating refresh token: %w", tokenErr)
		}
		refreshExpiresAt := now.Add(30 * 24 * time.Hour)
		_, queryErr = q.RotateSessionRefresh(ctx, db.RotateSessionRefreshParams{
			ID: session.ID, RefreshTokenHash: helpers.HashSecret(newRefresh), ExpiresAt: timestamp(refreshExpiresAt),
			RefreshTokenHash_2: helpers.HashSecret(refreshToken), ExpiresAt_2: timestamp(now),
		})
		if queryErr != nil {
			return apperror.ErrUnauthorized
		}
		access, tokenErr := s.signAccessToken(toEntity(user), uuidFromPG(session.ID), now)
		if tokenErr != nil {
			return tokenErr
		}
		result = dto.AuthData{
			Tokens: tokenData(tokenPair{
				AccessToken:      access,
				AccessExpiresAt:  now.Add(time.Duration(s.cfg.JWTExpiration) * time.Second),
				RefreshToken:     newRefresh,
				RefreshExpiresAt: refreshExpiresAt,
			}),
			User: SafeUser(toEntity(user)),
		}
		return nil
	})
	return result, err
}

func (s *Service) CurrentUser(ctx context.Context, userID uuid.UUID) (dto.UserData, error) {
	user, err := s.queries.GetActiveUser(ctx, db.GetActiveUserParams{ID: pgUUID(userID)})
	if err != nil {
		return dto.UserData{}, apperror.ErrUnauthorized
	}
	return SafeUser(toEntity(user)), nil
}

func (s *Service) UpdateCurrentUser(ctx context.Context, userID uuid.UUID, req dto.UpdateProfileRequest) (dto.UserData, error) {
	if req.FirstName == nil && req.LastName == nil && req.AvatarURL == nil && req.Language == nil {
		return dto.UserData{}, apperror.ErrInvalidData
	}

	firstName, err := optionalName(req.FirstName)
	if err != nil {
		return dto.UserData{}, err
	}
	lastName, err := optionalName(req.LastName)
	if err != nil {
		return dto.UserData{}, err
	}
	avatarURL, err := optionalAvatarURL(req.AvatarURL)
	if err != nil {
		return dto.UserData{}, err
	}
	language, err := optionalLanguage(req.Language)
	if err != nil {
		return dto.UserData{}, err
	}

	user, err := s.queries.UpdateUserProfile(ctx, db.UpdateUserProfileParams{
		FirstName: firstName,
		LastName:  lastName,
		AvatarUrl: avatarURL,
		Language:  language,
		UpdatedAt: timestamp(s.now().UTC()),
		ID:        pgUUID(userID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return dto.UserData{}, apperror.ErrUnauthorized
	}
	if err != nil {
		return dto.UserData{}, fmt.Errorf("updating current user: %w", err)
	}
	return SafeUser(toEntity(user)), nil
}

func (s *Service) ForgotPassword(ctx context.Context, phone string) (dto.StartData, error) {
	phone, err := helpers.NormalizePhone(phone)
	if err != nil {
		return dto.StartData{}, apperror.ErrInvalidData
	}
	user, err := s.queries.GetUserByPhone(ctx, db.GetUserByPhoneParams{Phone: text(phone)})
	if errors.Is(err, pgx.ErrNoRows) {
		return s.createOTPFlow(ctx, passwordResetPurpose, phone, uuid.Nil)
	}
	if err != nil {
		return dto.StartData{}, fmt.Errorf("finding password reset user: %w", err)
	}
	return s.createOTPFlow(ctx, passwordResetPurpose, phone, uuidFromPG(user.ID))
}

func (s *Service) VerifyPasswordOTP(ctx context.Context, phone, code string) (dto.ResetVerifyData, error) {
	phone, err := helpers.NormalizePhone(phone)
	if err != nil || !helpers.ValidOTP(code) || s.consumeOTP(ctx, passwordResetPurpose, phone, code) != nil {
		return dto.ResetVerifyData{}, apperror.ErrInvalidOTP
	}
	userID, err := s.flowUserID(ctx, passwordResetPurpose, phone)
	if err != nil || userID == uuid.Nil {
		return dto.ResetVerifyData{}, apperror.ErrInvalidOTP
	}
	token, err := helpers.RandomToken(32)
	if err != nil {
		return dto.ResetVerifyData{}, fmt.Errorf("creating reset token: %w", err)
	}
	expires := s.now().UTC().Add(10 * time.Minute)
	if err := s.otp.Set(ctx, "reset:"+helpers.HashSecret(token), userID.String(), time.Until(expires)); err != nil {
		return dto.ResetVerifyData{}, err
	}
	return dto.ResetVerifyData{ResetToken: token, ExpiresAt: expires.Format(time.RFC3339)}, nil
}

func (s *Service) ResetPassword(ctx context.Context, req dto.ResetPasswordRequest) error {
	if req.Password == "" || len(req.Password) < 8 {
		return apperror.ErrInvalidData
	}
	value, err := s.otp.Take(ctx, "reset:"+helpers.HashSecret(req.ResetToken))
	if err != nil {
		return apperror.ErrInvalidResetToken
	}
	userID, err := uuid.Parse(value)
	if err != nil {
		return apperror.ErrInvalidResetToken
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hashing reset password: %w", err)
	}
	return s.withTx(ctx, func(q *db.Queries) error {
		now := timestamp(s.now().UTC())
		rows, updateErr := q.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{ID: pgUUID(userID), PasswordHash: text(string(hash)), UpdatedAt: now})
		if updateErr != nil || rows != 1 {
			return apperror.ErrInvalidResetToken
		}
		return q.RevokeActiveSession(ctx, db.RevokeActiveSessionParams{UserID: pgUUID(userID), RevokedAt: now})
	})
}

func (s *Service) Logout(ctx context.Context, userID, sessionID uuid.UUID) error {
	rows, err := s.queries.RevokeSession(ctx, db.RevokeSessionParams{ID: pgUUID(sessionID), UserID: pgUUID(userID), RevokedAt: timestamp(s.now().UTC())})
	if err != nil {
		return fmt.Errorf("revoking session: %w", err)
	}
	if rows != 1 {
		return apperror.ErrSessionRevoked
	}
	return nil
}

func (s *Service) PhoneChangeRequest(ctx context.Context, userID uuid.UUID, newPhone string) (dto.StartData, error) {
	phone, err := helpers.NormalizePhone(newPhone)
	if err != nil {
		return dto.StartData{}, apperror.ErrInvalidData
	}
	exists, err := s.queries.UserPhoneExistsOther(ctx, db.UserPhoneExistsOtherParams{Phone: text(phone), ID: pgUUID(userID)})
	if err != nil {
		return dto.StartData{}, fmt.Errorf("checking phone availability: %w", err)
	}
	if exists {
		return dto.StartData{}, apperror.ErrAlreadyExists
	}
	return s.createOTPFlow(ctx, phoneChangePurpose, phone, userID)
}

func (s *Service) PhoneChangeConfirm(ctx context.Context, userID uuid.UUID, newPhone, code string) error {
	phone, err := helpers.NormalizePhone(newPhone)
	if err != nil || !helpers.ValidOTP(code) || s.consumeOTP(ctx, phoneChangePurpose, phone, code) != nil {
		return apperror.ErrInvalidOTP
	}
	storedID, err := s.flowUserID(ctx, phoneChangePurpose, phone)
	if err != nil || storedID != userID {
		return apperror.ErrInvalidOTP
	}
	rows, err := s.queries.UpdateUserPhone(ctx, db.UpdateUserPhoneParams{ID: pgUUID(userID), Phone: text(phone), UpdatedAt: timestamp(s.now().UTC())})
	if err != nil {
		return fmt.Errorf("updating phone: %w", err)
	}
	if rows != 1 {
		return apperror.ErrUnauthorized
	}
	return nil
}

func (s *Service) HandleBotStart(ctx context.Context, startToken string, chatID int64) error {
	value, err := s.otp.Take(ctx, "start:"+helpers.HashSecret(startToken))
	if err != nil {
		return apperror.ErrInvalidOTP
	}
	parts := strings.Split(value, "\n")
	if len(parts) != 2 {
		return apperror.ErrInvalidOTP
	}
	purpose, phone := parts[0], parts[1]
	code, err := s.generateOTP()
	if err != nil {
		return fmt.Errorf("generating otp: %w", err)
	}
	ttl := time.Duration(s.cfg.OTPExpiration) * time.Second
	if err := s.otp.Set(ctx, s.codeKey(purpose, phone), s.hashOTP(code), ttl); err != nil {
		return err
	}
	if err := s.otp.Set(ctx, s.chatKey(purpose, phone), strconv.FormatInt(chatID, 10), ttl); err != nil {
		return err
	}
	if s.otpSender == nil {
		return apperror.ErrTelegramUnavailable
	}
	if err := s.otpSender.SendMessage(ctx, chatID, "Ваш код подтверждения: "+code); err != nil {
		return fmt.Errorf("sending telegram otp: %w", err)
	}
	if _, err := s.otp.Reserve(ctx, s.cooldownKey(purpose, phone), time.Duration(s.cfg.OTPResendCooldown)*time.Second); err != nil {
		return err
	}
	return nil
}

func (s *Service) Resend(ctx context.Context, purpose, phone string, userID *uuid.UUID) error {
	phone, err := helpers.NormalizePhone(phone)
	if err != nil {
		return apperror.ErrInvalidData
	}
	if userID != nil {
		storedID, loadErr := s.flowUserID(ctx, purpose, phone)
		if loadErr != nil || storedID != *userID {
			return apperror.ErrBotNotStarted
		}
	}
	chatValue, err := s.otp.Get(ctx, s.chatKey(purpose, phone))
	if err != nil {
		return apperror.ErrBotNotStarted
	}
	chatID, err := strconv.ParseInt(chatValue, 10, 64)
	if err != nil {
		return apperror.ErrBotNotStarted
	}
	reserved, err := s.otp.Reserve(ctx, s.cooldownKey(purpose, phone), time.Duration(s.cfg.OTPResendCooldown)*time.Second)
	if err != nil {
		return err
	}
	if !reserved {
		return apperror.ErrLimitExceeded
	}
	code, err := s.generateOTP()
	if err != nil {
		return fmt.Errorf("generating otp: %w", err)
	}
	ttl := time.Duration(s.cfg.OTPExpiration) * time.Second
	if err := s.otp.Set(ctx, s.codeKey(purpose, phone), s.hashOTP(code), ttl); err != nil {
		return err
	}
	if s.otpSender == nil {
		return apperror.ErrTelegramUnavailable
	}
	if err := s.otpSender.SendMessage(ctx, chatID, "Ваш новый код подтверждения: "+code); err != nil {
		return fmt.Errorf("sending telegram otp: %w", err)
	}
	return nil
}

type tokenPair struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
}

func tokenData(pair tokenPair) dto.TokenData {
	return dto.TokenData{
		AccessToken:      pair.AccessToken,
		AccessExpiresAt:  pair.AccessExpiresAt.Format(time.RFC3339),
		RefreshToken:     pair.RefreshToken,
		RefreshExpiresAt: pair.RefreshExpiresAt.Format(time.RFC3339),
	}
}

func (s *Service) issueSession(ctx context.Context, q *db.Queries, user db.User) (tokenPair, error) {
	now := s.now().UTC()
	accessExpiresAt := now.Add(time.Duration(s.cfg.JWTExpiration) * time.Second)
	refreshExpiresAt := now.Add(30 * 24 * time.Hour)
	refresh, err := helpers.RandomToken(32)
	if err != nil {
		return tokenPair{}, fmt.Errorf("creating refresh token: %w", err)
	}
	if err := q.RevokeActiveSession(ctx, db.RevokeActiveSessionParams{UserID: user.ID, RevokedAt: timestamp(now)}); err != nil {
		return tokenPair{}, fmt.Errorf("revoking previous session: %w", err)
	}
	sessionID := uuid.New()
	if _, err := q.CreateSession(ctx, db.CreateSessionParams{ID: pgUUID(sessionID), UserID: user.ID, RefreshTokenHash: helpers.HashSecret(refresh), ExpiresAt: timestamp(refreshExpiresAt), CreatedAt: timestamp(now)}); err != nil {
		return tokenPair{}, fmt.Errorf("creating session: %w", err)
	}
	access, err := s.signAccessToken(toEntity(user), sessionID, now)
	if err != nil {
		return tokenPair{}, err
	}
	return tokenPair{
		AccessToken: access, AccessExpiresAt: accessExpiresAt,
		RefreshToken: refresh, RefreshExpiresAt: refreshExpiresAt,
	}, nil
}

func (s *Service) signAccessToken(user entities.User, sessionID uuid.UUID, now time.Time) (string, error) {
	claims := jwt.MapClaims{"sub": user.ID.String(), "sid": sessionID.String(), "type": "access", "iss": s.cfg.JWTIssuer, "aud": s.cfg.JWTAudience, "iat": now.Unix(), "exp": now.Add(time.Duration(s.cfg.JWTExpiration) * time.Second).Unix()}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return "", fmt.Errorf("signing access token: %w", err)
	}
	return token, nil
}

func (s *Service) createOTPFlow(ctx context.Context, purpose, phone string, userID uuid.UUID) (dto.StartData, error) {
	token, err := helpers.RandomToken(32)
	if err != nil {
		return dto.StartData{}, fmt.Errorf("creating telegram start token: %w", err)
	}
	expires := s.now().UTC().Add(time.Duration(s.cfg.OTPExpiration) * time.Second)
	ttl := time.Until(expires)
	if err := s.otp.Set(ctx, "start:"+helpers.HashSecret(token), purpose+"\n"+phone, ttl); err != nil {
		return dto.StartData{}, err
	}
	if userID != uuid.Nil {
		if err := s.otp.Set(ctx, s.subjectKey(purpose, phone), userID.String(), ttl); err != nil {
			return dto.StartData{}, err
		}
	}
	if code := s.configuredOTP(); code != "" {
		if err := s.otp.Set(ctx, s.codeKey(purpose, phone), s.hashOTP(code), ttl); err != nil {
			return dto.StartData{}, err
		}
	}
	return s.startData(token, expires), nil
}

func (s *Service) consumeOTP(ctx context.Context, purpose, phone, code string) error {
	valid, err := s.otp.Verify(ctx, s.codeKey(purpose, phone), s.hashOTP(code), s.cfg.OTPMaxAttempts)
	if err != nil || !valid {
		return apperror.ErrInvalidOTP
	}
	return nil
}

func (s *Service) flowUserID(ctx context.Context, purpose, phone string) (uuid.UUID, error) {
	value, err := s.otp.Get(ctx, s.subjectKey(purpose, phone))
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(value)
}

func (s *Service) subjectKey(purpose, phone string) string { return "subject:" + purpose + ":" + phone }
func (s *Service) codeKey(purpose, phone string) string    { return "code:" + purpose + ":" + phone }
func (s *Service) chatKey(purpose, phone string) string    { return "chat:" + purpose + ":" + phone }
func (s *Service) cooldownKey(purpose, phone string) string {
	return "cooldown:" + purpose + ":" + phone
}

func (s *Service) startData(token string, expires time.Time) dto.StartData {
	link := ""
	if s.cfg.TelegramBotUsername != "" {
		link = "https://t.me/" + strings.TrimPrefix(s.cfg.TelegramBotUsername, "@") + "?start=" + token
	}
	return dto.StartData{
		TelegramDeepLink: link,
		ExpiresAt:        expires.Format(time.RFC3339),
		TTL:              s.cfg.OTPExpiration,
		ResendIn:         s.cfg.OTPResendCooldown,
	}
}

func (s *Service) hashOTP(code string) string { return helpers.HashHMAC(code, s.cfg.OTPPepper) }

func (s *Service) configuredOTP() string {
	code := strings.TrimSpace(s.cfg.OTPDefaultCode)
	if helpers.ValidOTP(code) {
		return code
	}
	return ""
}

func (s *Service) generateOTP() (string, error) {
	if code := s.configuredOTP(); code != "" {
		return code, nil
	}
	return helpers.GenerateOTP()
}

func (s *Service) withTx(ctx context.Context, fn func(*db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.queries.WithTx(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func text(value string) pgtype.Text { return pgtype.Text{String: value, Valid: true} }
func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func pgUUID(value uuid.UUID) pgtype.UUID     { return pgtype.UUID{Bytes: value, Valid: true} }
func uuidFromPG(value pgtype.UUID) uuid.UUID { return uuid.UUID(value.Bytes) }

func toEntity(user db.User) entities.User {
	result := entities.User{
		ID: uuidFromPG(user.ID), Password: user.Password.String, PasswordHash: user.PasswordHash.String,
		Name: user.Name.String, Phone: user.Phone.String, Username: user.Username.String, FirstName: user.FirstName.String,
		LastName: user.LastName.String, AvatarURL: user.AvatarUrl.String, Language: user.Language, IsActive: user.IsActive,
		CreatedAt: user.CreatedAt.Time, UpdatedAt: user.UpdatedAt.Time,
	}
	if user.LastLoginAt.Valid {
		result.LastLoginAt = &user.LastLoginAt.Time
	}
	return result
}

func SafeUser(user entities.User) dto.UserData {
	return dto.UserData{
		ID: user.ID, Phone: user.Phone, Username: user.Username, FirstName: user.FirstName,
		LastName: user.LastName, AvatarURL: user.AvatarURL, Language: user.Language,
		IsActive: user.IsActive, LastLoginAt: user.LastLoginAt, CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}

func optionalName(value *string) (pgtype.Text, error) {
	if value == nil {
		return pgtype.Text{}, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" || len([]rune(trimmed)) > 100 {
		return pgtype.Text{}, apperror.ErrInvalidData
	}
	return text(trimmed), nil
}

func optionalLanguage(value *string) (pgtype.Text, error) {
	if value == nil {
		return pgtype.Text{}, nil
	}
	language := strings.ToLower(strings.TrimSpace(*value))
	if language != "uz" && language != "ru" && language != "en" {
		return pgtype.Text{}, apperror.ErrInvalidData
	}
	return text(language), nil
}

func optionalAvatarURL(value *string) (pgtype.Text, error) {
	if value == nil {
		return pgtype.Text{}, nil
	}
	avatar := strings.TrimSpace(*value)
	if avatar == "" {
		return text(""), nil
	}
	if len(avatar) > 2048 {
		return pgtype.Text{}, apperror.ErrInvalidData
	}
	parsed, err := url.ParseRequestURI(avatar)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return pgtype.Text{}, apperror.ErrInvalidData
	}
	return text(avatar), nil
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
