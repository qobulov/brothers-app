package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	dto "github.com/qobulov/brothers-app/internal/auth/dto"
	"github.com/qobulov/brothers-app/internal/auth/otp"
	db "github.com/qobulov/brothers-app/internal/db"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/helpers"
)

// EmailChangePurpose is the OTP purpose for changing a logged-in user's email.
// The code goes to the new address, which proves the user owns it.
const EmailChangePurpose = "email_change"

// SendEmailChangeOTP sends a code to the new email address of a logged-in user.
func (s *Service) SendEmailChangeOTP(ctx context.Context, userID uuid.UUID, newEmail string) (dto.StartData, error) {
	email, err := s.newEmailFor(ctx, userID, newEmail)
	if err != nil {
		return dto.StartData{}, err
	}
	return s.createOTPFlow(ctx, EmailChangePurpose, email, userID)
}

// ChangeEmail switches the user's email once the code sent to the new address
// is confirmed. The code is bound to this user: an OTP requested by someone
// else for the same address does not work. The current session stays valid.
func (s *Service) ChangeEmail(ctx context.Context, userID uuid.UUID, req dto.ChangeEmailRequest) (dto.UserData, error) {
	email, err := s.newEmailFor(ctx, userID, req.NewEmail)
	if err != nil {
		return dto.UserData{}, err
	}
	if !helpers.ValidOTP(req.OTPCode) {
		return dto.UserData{}, apperror.ErrInvalidOTP
	}
	requester, err := s.flowUserID(ctx, EmailChangePurpose, email)
	if errors.Is(err, otp.ErrNotFound) || (err == nil && requester != userID) {
		return dto.UserData{}, apperror.ErrInvalidOTP
	}
	if err != nil {
		return dto.UserData{}, fmt.Errorf("loading email change request: %w", err)
	}
	if err := s.consumeOTP(ctx, EmailChangePurpose, email, req.OTPCode); err != nil {
		return dto.UserData{}, err
	}
	user, err := s.queries.UpdateUserEmail(ctx, db.UpdateUserEmailParams{
		ID: pgUUID(userID), Email: text(email), UpdatedAt: timestamp(s.now().UTC()),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return dto.UserData{}, apperror.ErrUnauthorized
	}
	if isUniqueViolation(err) {
		return dto.UserData{}, emailTaken()
	}
	if err != nil {
		return dto.UserData{}, fmt.Errorf("changing email: %w", err)
	}
	// The change is done; a leftover request key only expires on its own.
	_ = s.otp.Delete(ctx, s.subjectKey(EmailChangePurpose, email))
	return SafeUser(toEntity(user)), nil
}

// newEmailFor normalizes the requested address and checks that it differs from
// the user's current email and is not used by another account.
func (s *Service) newEmailFor(ctx context.Context, userID uuid.UUID, value string) (string, error) {
	email, err := helpers.NormalizeEmail(value)
	if err != nil {
		return "", apperror.InvalidEmail()
	}
	current, err := s.queries.GetActiveUser(ctx, db.GetActiveUserParams{ID: pgUUID(userID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperror.ErrUnauthorized
	}
	if err != nil {
		return "", fmt.Errorf("loading user for email change: %w", err)
	}
	if strings.EqualFold(current.Email.String, email) {
		return "", apperror.New(apperror.ErrInvalidData, apperror.Text{
			UZ: "Yangi email hozirgisi bilan bir xil",
			RU: "Новый email совпадает с текущим",
			EN: "The new email is the same as the current one",
		})
	}
	other, err := s.queries.GetUserByEmail(ctx, db.GetUserByEmailParams{Email: text(email)})
	if err == nil && uuidFromPG(other.ID) != userID {
		return "", emailTaken()
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("checking new email: %w", err)
	}
	return email, nil
}

func emailTaken() error {
	return apperror.New(apperror.ErrAlreadyExists, apperror.Text{
		UZ: "Bu email boshqa akkauntda ishlatilgan",
		RU: "Этот email уже используется другим аккаунтом",
		EN: "This email is already used by another account",
	})
}
