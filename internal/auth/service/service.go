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
	sessionpkg "github.com/qobulov/brothers-app/internal/auth/session"
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
	defaultOTP           = "111111"
)

type EmailSender interface {
	SendOTP(ctx context.Context, recipient, code string) error
}

type Service struct {
	pool        *pgxpool.Pool
	queries     *db.Queries
	otp         *otp.Cache
	cfg         *config.Config
	emailSender EmailSender
	sessions    sessionpkg.Store
	now         func() time.Time
}

func New(pool *pgxpool.Pool, otpCache *otp.Cache, sessions sessionpkg.Store, cfg *config.Config, sender EmailSender) *Service {
	return &Service{pool: pool, queries: db.New(pool), otp: otpCache, sessions: sessions, cfg: cfg, emailSender: sender, now: time.Now}
}

func (s *Service) SendOTP(ctx context.Context, req dto.SendOTPRequest) (dto.StartData, error) {
	purpose, err := normalizeOTPPurpose(req.Purpose)
	if err != nil {
		return dto.StartData{}, err
	}

	var email string
	var userID uuid.UUID
	switch purpose {
	case registrationPurpose:
		if strings.TrimSpace(req.Username) != "" {
			return dto.StartData{}, apperror.New(apperror.ErrInvalidData, apperror.Text{UZ: "Ro'yxatdan o'tishda faqat email yuboriladi, username yuborilmasligi kerak", RU: "При регистрации отправляется только email, username указывать не нужно", EN: "Registration accepts email only; omit the username"})
		}
		if strings.TrimSpace(req.Email) == "" {
			return dto.StartData{}, apperror.New(apperror.ErrInvalidData, apperror.Text{UZ: "Ro'yxatdan o'tish uchun email kiritilishi shart", RU: "Для регистрации необходим email", EN: "Email is required for registration"})
		}
		email, err = helpers.NormalizeEmail(req.Email)
		if err != nil {
			return dto.StartData{}, invalidEmail()
		}
		_, err = s.queries.GetUserByEmail(ctx, db.GetUserByEmailParams{Email: text(email)})
		if err == nil {
			return dto.StartData{}, apperror.ErrRegistrationIdentityExists
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return dto.StartData{}, fmt.Errorf("checking registration email: %w", err)
		}
	case passwordResetPurpose:
		lookupEmail, username, lookupErr := passwordResetIdentifier(req)
		if lookupErr != nil {
			return dto.StartData{}, lookupErr
		}

		var user db.User
		var findErr error
		if username != "" {
			user, findErr = s.queries.GetUserByLogin(ctx, db.GetUserByLoginParams{Username: text(username), Email: text("")})
		} else {
			user, findErr = s.queries.GetUserByEmail(ctx, db.GetUserByEmailParams{Email: text(lookupEmail)})
		}
		if findErr == nil {
			email, err = helpers.NormalizeEmail(user.Email.String)
			if err != nil {
				return dto.StartData{}, apperror.ErrInvalidData
			}
			userID = uuidFromPG(user.ID)
		} else if errors.Is(findErr, pgx.ErrNoRows) {
			expires := s.now().UTC().Add(time.Duration(s.cfg.OTPExpiration) * time.Second)
			return s.startData(expires), nil
		} else {
			return dto.StartData{}, fmt.Errorf("finding password reset user by email: %w", findErr)
		}
	}
	return s.createOTPFlow(ctx, purpose, email, userID)
}

// passwordResetIdentifier accepts either an email address or a username, but
// never both. The resolved account email is later used as the OTP recipient
// and as the OTP cache key.
func passwordResetIdentifier(req dto.SendOTPRequest) (email, username string, err error) {
	emailInput := strings.TrimSpace(req.Email)
	username = strings.TrimSpace(req.Username)
	if emailInput == "" && username == "" {
		return "", "", apperror.New(apperror.ErrInvalidData, apperror.Text{UZ: "Parolni tiklash uchun email yoki username kiriting", RU: "Для сброса пароля укажите email или username", EN: "Enter an email or a username to reset the password"})
	}
	if emailInput != "" && username != "" {
		return "", "", apperror.New(apperror.ErrInvalidData, apperror.Text{UZ: "Email yoki username'dan faqat bittasini kiriting", RU: "Укажите только email или только username", EN: "Provide either an email or a username, not both"})
	}
	if username != "" {
		return "", username, nil
	}
	email, err = helpers.NormalizeEmail(emailInput)
	if err != nil {
		return "", "", invalidEmail()
	}
	return email, "", nil
}

func (s *Service) Register(ctx context.Context, req dto.RegisterRequest) (dto.RegisterData, error) {
	email, err := helpers.NormalizeEmail(req.Email)
	if err != nil {
		return dto.RegisterData{}, apperror.ErrInvalidData
	}
	phone := pgtype.Text{}
	if strings.TrimSpace(req.Phone) != "" {
		normalizedPhone, normalizeErr := helpers.NormalizePhone(req.Phone)
		if normalizeErr != nil {
			return dto.RegisterData{}, apperror.ErrInvalidData
		}
		phone = text(normalizedPhone)
	}
	username := strings.TrimSpace(req.Username)
	firstName := strings.TrimSpace(req.FirstName)
	lastName := strings.TrimSpace(req.LastName)
	if req.Password == "" || username == "" || firstName == "" || len(req.Password) < 8 || len(username) > 50 {
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
		_, findErr := q.GetUserByEmailOrUsername(ctx, db.GetUserByEmailOrUsernameParams{Email: text(email), Username: text(username)})
		if findErr == nil {
			return apperror.ErrRegistrationIdentityExists
		}
		if !errors.Is(findErr, pgx.ErrNoRows) {
			return fmt.Errorf("checking registration identity: %w", findErr)
		}
		if err := s.consumeOTP(ctx, registrationPurpose, email, req.OTPCode); err != nil {
			return err
		}

		now := timestamp(s.now().UTC())
		user, findErr := q.CreateAuthUser(ctx, db.CreateAuthUserParams{
			ID:           pgUUID(uuid.New()),
			PasswordHash: text(string(passwordHash)),
			Name:         text(strings.TrimSpace(firstName + " " + lastName)),
			Email:        text(email),
			Phone:        phone,
			Username:     text(username),
			FirstName:    text(firstName),
			LastName:     text(lastName),
			AvatarUrl:    text(req.AvatarURL),
			Language:     req.Language,
			CreatedAt:    now,
		})
		if findErr != nil {
			return fmt.Errorf("saving registration user: %w", findErr)
		}
		pair, issueErr := s.issueSession(ctx, user)
		if issueErr != nil {
			return issueErr
		}
		result = dto.RegisterData{
			Tokens: tokenData(pair),
			User: dto.RegisterUserData{
				ID: uuidFromPG(user.ID), FullName: user.Name.String, Email: user.Email.String, Phone: user.Phone.String, Role: "user",
			},
		}
		return nil
	})
	if isUniqueViolation(err) {
		return dto.RegisterData{}, fmt.Errorf("%w: %w", apperror.ErrRegistrationIdentityExists, err)
	}
	return result, err
}

func (s *Service) Login(ctx context.Context, login, password string) (dto.AuthData, error) {
	login, email := loginIdentifiers(login)
	if login == "" || password == "" {
		return dto.AuthData{}, apperror.ErrInvalidCredentials
	}
	// bcrypt runs outside any transaction so a login never holds a pool
	// connection or row lock while hashing.
	user, err := s.queries.GetUserByLogin(ctx, db.GetUserByLoginParams{Username: text(login), Email: text(email)})
	if errors.Is(err, pgx.ErrNoRows) {
		// Hash anyway so unknown accounts take as long as wrong passwords.
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(password))
		return dto.AuthData{}, apperror.ErrInvalidCredentials
	}
	if err != nil {
		return dto.AuthData{}, fmt.Errorf("finding login user: %w", err)
	}
	if !user.IsActive || !passwordMatches(user, password) {
		return dto.AuthData{}, apperror.ErrInvalidCredentials
	}
	user, err = s.queries.UpdateUserLogin(ctx, db.UpdateUserLoginParams{ID: user.ID, LastLoginAt: timestamp(s.now().UTC())})
	if errors.Is(err, pgx.ErrNoRows) {
		// The account was deactivated or deleted after the password check.
		return dto.AuthData{}, apperror.ErrInvalidCredentials
	}
	if err != nil {
		return dto.AuthData{}, fmt.Errorf("updating login time: %w", err)
	}
	pair, err := s.issueSession(ctx, user)
	if err != nil {
		return dto.AuthData{}, err
	}
	return dto.AuthData{Tokens: tokenData(pair), User: SafeUser(toEntity(user))}, nil
}

// dummyPasswordHash is a bcrypt hash of a discarded random value, used to
// equalize login timing for unknown accounts.
var dummyPasswordHash = []byte("$2a$10$BhiSASoMqxbz6RqSG.E7P.aooDeyhnD/tvOQtOqwrseAslEcBziIq")

func passwordMatches(user db.User, password string) bool {
	hash := user.PasswordHash.String
	if hash == "" {
		hash = user.Password.String
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func loginIdentifiers(value string) (username, email string) {
	username = strings.TrimSpace(value)
	if normalized, err := helpers.NormalizeEmail(username); err == nil {
		email = normalized
	}
	return username, email
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (dto.AuthData, error) {
	if refreshToken == "" {
		return dto.AuthData{}, apperror.ErrUnauthorized
	}
	if s.sessions == nil {
		return dto.AuthData{}, fmt.Errorf("session store is not configured")
	}
	now := s.now().UTC()
	newRefresh, err := helpers.RandomToken(32)
	if err != nil {
		return dto.AuthData{}, fmt.Errorf("creating refresh token: %w", err)
	}
	refreshExpiresAt := now.Add(30 * 24 * time.Hour)
	session, err := s.sessions.Rotate(ctx, helpers.HashSecret(refreshToken), helpers.HashSecret(newRefresh), refreshExpiresAt.Sub(now))
	if errors.Is(err, sessionpkg.ErrNotFound) {
		return dto.AuthData{}, apperror.ErrUnauthorized
	}
	if err != nil {
		return dto.AuthData{}, fmt.Errorf("rotating refresh session: %w", err)
	}
	user, err := s.queries.GetActiveUser(ctx, db.GetActiveUserParams{ID: pgUUID(session.UserID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return dto.AuthData{}, apperror.ErrUnauthorized
	}
	if err != nil {
		return dto.AuthData{}, fmt.Errorf("loading refresh user: %w", err)
	}
	access, err := s.signAccessToken(toEntity(user), session.SessionID, now)
	if err != nil {
		return dto.AuthData{}, err
	}
	return dto.AuthData{
		Tokens: tokenData(tokenPair{
			AccessToken:      access,
			AccessExpiresAt:  now.Add(time.Duration(s.cfg.JWTExpiration) * time.Second),
			RefreshToken:     newRefresh,
			RefreshExpiresAt: refreshExpiresAt,
		}),
		User: SafeUser(toEntity(user)),
	}, nil
}

func (s *Service) CurrentUser(ctx context.Context, userID uuid.UUID) (dto.UserData, error) {
	user, err := s.queries.GetActiveUser(ctx, db.GetActiveUserParams{ID: pgUUID(userID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return dto.UserData{}, apperror.ErrUnauthorized
	}
	if err != nil {
		return dto.UserData{}, fmt.Errorf("loading current user: %w", err)
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

func (s *Service) VerifyPasswordOTP(ctx context.Context, email, code string) (dto.ResetVerifyData, error) {
	email, err := helpers.NormalizeEmail(email)
	if err != nil || !helpers.ValidOTP(code) {
		return dto.ResetVerifyData{}, apperror.ErrInvalidOTP
	}
	if err := s.consumeOTP(ctx, passwordResetPurpose, email, code); err != nil {
		return dto.ResetVerifyData{}, err
	}
	userID, err := s.flowUserID(ctx, passwordResetPurpose, email)
	if errors.Is(err, otp.ErrNotFound) {
		if !defaultOTPEnabled() || code != defaultOTP {
			return dto.ResetVerifyData{}, apperror.ErrInvalidOTP
		}
		user, findErr := s.queries.GetUserByEmail(ctx, db.GetUserByEmailParams{Email: text(email)})
		if errors.Is(findErr, pgx.ErrNoRows) {
			return dto.ResetVerifyData{}, s.passwordResetUserNotFound(email)
		}
		if findErr != nil {
			return dto.ResetVerifyData{}, fmt.Errorf("loading default password reset subject: %w", findErr)
		}
		userID = uuidFromPG(user.ID)
		err = nil
	}
	if err != nil {
		return dto.ResetVerifyData{}, fmt.Errorf("loading password reset subject: %w", err)
	}
	if userID == uuid.Nil {
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
	if errors.Is(err, otp.ErrNotFound) {
		return apperror.ErrInvalidResetToken
	}
	if err != nil {
		return fmt.Errorf("loading password reset token: %w", err)
	}
	userID, err := uuid.Parse(value)
	if err != nil {
		return apperror.ErrInvalidResetToken
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hashing reset password: %w", err)
	}
	rows, err := s.queries.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{ID: pgUUID(userID), PasswordHash: text(string(hash)), UpdatedAt: timestamp(s.now().UTC())})
	if err != nil {
		return fmt.Errorf("updating password: %w", err)
	}
	if rows != 1 {
		return apperror.ErrInvalidResetToken
	}
	if s.sessions == nil {
		return fmt.Errorf("session store is not configured")
	}
	if err := s.sessions.RevokeUser(ctx, userID); err != nil {
		return fmt.Errorf("revoking redis sessions after password reset: %w", err)
	}
	return nil
}

func (s *Service) Logout(ctx context.Context, userID, sessionID uuid.UUID) error {
	if s.sessions == nil {
		return fmt.Errorf("session store is not configured")
	}
	revoked, err := s.sessions.Revoke(ctx, userID, sessionID)
	if err != nil {
		return fmt.Errorf("revoking redis session: %w", err)
	}
	if !revoked {
		return apperror.ErrSessionRevoked
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

func (s *Service) issueSession(ctx context.Context, user db.User) (tokenPair, error) {
	now := s.now().UTC()
	accessExpiresAt := now.Add(time.Duration(s.cfg.JWTExpiration) * time.Second)
	refreshExpiresAt := now.Add(30 * 24 * time.Hour)
	refresh, err := helpers.RandomToken(32)
	if err != nil {
		return tokenPair{}, fmt.Errorf("creating refresh token: %w", err)
	}
	sessionID := uuid.New()
	if s.sessions == nil {
		return tokenPair{}, fmt.Errorf("session store is not configured")
	}
	if err := s.sessions.Create(ctx, sessionpkg.Data{UserID: uuidFromPG(user.ID), SessionID: sessionID, RefreshTokenHash: helpers.HashSecret(refresh)}, refreshExpiresAt.Sub(now)); err != nil {
		return tokenPair{}, fmt.Errorf("creating redis session: %w", err)
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

func (s *Service) createOTPFlow(ctx context.Context, purpose, email string, userID uuid.UUID) (dto.StartData, error) {
	code, err := s.generateOTP()
	if err != nil {
		return dto.StartData{}, fmt.Errorf("generating otp: %w", err)
	}
	reserved, err := s.otp.Reserve(ctx, s.cooldownKey(purpose, email), time.Duration(s.cfg.OTPResendCooldown)*time.Second)
	if err != nil {
		return dto.StartData{}, err
	}
	if !reserved {
		return dto.StartData{}, apperror.ErrLimitExceeded
	}
	expires := s.now().UTC().Add(time.Duration(s.cfg.OTPExpiration) * time.Second)
	ttl := time.Until(expires)
	if err := s.otp.Set(ctx, s.codeKey(purpose, email), s.hashOTP(code), ttl); err != nil {
		if cleanupErr := s.otp.Delete(ctx, s.cooldownKey(purpose, email)); cleanupErr != nil {
			return dto.StartData{}, errors.Join(err, cleanupErr)
		}
		return dto.StartData{}, err
	}
	if userID != uuid.Nil {
		if err := s.otp.Set(ctx, s.subjectKey(purpose, email), userID.String(), ttl); err != nil {
			cleanupErr := errors.Join(
				s.otp.Delete(ctx, s.codeKey(purpose, email)),
				s.otp.Delete(ctx, s.cooldownKey(purpose, email)),
			)
			if cleanupErr != nil {
				return dto.StartData{}, errors.Join(err, cleanupErr)
			}
			return dto.StartData{}, err
		}
	}
	if s.emailSender != nil {
		if err := s.emailSender.SendOTP(ctx, email, code); err != nil {
			// Formula's default OTP flow must stay usable while delivery is being
			// configured. A real email OTP remains cached if delivery succeeds.
			if defaultOTPEnabled() {
				return s.startData(expires, email), nil
			}
			cleanupErr := errors.Join(
				s.otp.Delete(ctx, s.codeKey(purpose, email)),
				s.otp.Delete(ctx, s.subjectKey(purpose, email)),
				s.otp.Delete(ctx, s.cooldownKey(purpose, email)),
			)
			if cleanupErr != nil {
				return dto.StartData{}, fmt.Errorf("sending email otp and cleaning up flow: %w", errors.Join(apperror.ErrEmailUnavailable, err, cleanupErr))
			}
			return dto.StartData{}, fmt.Errorf("%w: %w", apperror.ErrEmailUnavailable, err)
		}
	}
	return s.startData(expires, email), nil
}

func (s *Service) consumeOTP(ctx context.Context, purpose, email, code string) error {
	if defaultOTPEnabled() && code == defaultOTP {
		return nil
	}
	valid, err := s.otp.Verify(ctx, s.codeKey(purpose, email), s.hashOTP(code), s.cfg.OTPMaxAttempts)
	if err != nil {
		return fmt.Errorf("checking otp: %w", err)
	}
	if !valid {
		return apperror.ErrInvalidOTP
	}
	return nil
}

func (s *Service) flowUserID(ctx context.Context, purpose, email string) (uuid.UUID, error) {
	value, err := s.otp.Get(ctx, s.subjectKey(purpose, email))
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(value)
}

func (s *Service) subjectKey(purpose, email string) string { return "subject:" + purpose + ":" + email }
func (s *Service) codeKey(purpose, email string) string    { return "code:" + purpose + ":" + email }
func (s *Service) cooldownKey(purpose, email string) string {
	return "cooldown:" + purpose + ":" + email
}

func (s *Service) startData(expires time.Time, recipient ...string) dto.StartData {
	data := dto.StartData{
		ExpiresAt: expires.Format(time.RFC3339),
		TTL:       s.cfg.OTPExpiration,
		ResendIn:  s.cfg.OTPResendCooldown,
	}
	if len(recipient) > 0 {
		data.Email = recipient[0]
	}
	return data
}

func (s *Service) hashOTP(code string) string { return helpers.HashHMAC(code, s.cfg.OTPPepper) }

func normalizeOTPPurpose(value string) (string, error) {
	purpose := strings.ToLower(strings.TrimSpace(value))
	if purpose == "" {
		purpose = registrationPurpose
	}
	if purpose != registrationPurpose && purpose != passwordResetPurpose {
		return "", apperror.New(apperror.ErrInvalidData, apperror.Text{UZ: "OTP maqsadi registration yoki password_reset bo'lishi kerak", RU: "Цель OTP должна быть registration или password_reset", EN: "Purpose must be registration or password_reset"})
	}
	return purpose, nil
}

func (s *Service) generateOTP() (string, error) {
	return helpers.GenerateOTP()
}

func defaultOTPEnabled() bool { return defaultOTP != "" }

func (s *Service) passwordResetUserNotFound(email string) error {
	if s.cfg != nil && strings.EqualFold(strings.TrimSpace(s.cfg.AppEnv), "development") {
		return fmt.Errorf("%w: password reset user not found for email %q", apperror.ErrInvalidOTP, email)
	}
	return apperror.ErrInvalidOTP
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
		Name: user.Name.String, Email: user.Email.String, Phone: user.Phone.String, Username: user.Username.String, FirstName: user.FirstName.String,
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
		ID: user.ID, Email: user.Email, Phone: user.Phone, Username: user.Username, FirstName: user.FirstName,
		LastName: user.LastName, AvatarURL: user.AvatarURL, Language: user.Language,
		IsActive: user.IsActive, LastLoginAt: user.LastLoginAt,
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

func invalidEmail() error {
	return apperror.New(apperror.ErrInvalidData, apperror.Text{
		UZ: "Email manzili noto'g'ri",
		RU: "Некорректный email",
		EN: "Enter a valid email address",
	})
}
