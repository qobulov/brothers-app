package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	dto "github.com/qobulov/brothers-app/internal/auth/dto"
	db "github.com/qobulov/brothers-app/internal/db"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"golang.org/x/crypto/bcrypt"
)

// ChangePassword replaces a logged-in user's password after checking the
// current one. The current session stays valid.
func (s *Service) ChangePassword(ctx context.Context, userID uuid.UUID, req dto.ChangePasswordRequest) error {
	user, err := s.queries.GetActiveUser(ctx, db.GetActiveUserParams{ID: pgUUID(userID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return apperror.ErrUnauthorized
	}
	if err != nil {
		return fmt.Errorf("loading user for password change: %w", err)
	}
	if !passwordMatches(user, req.CurrentPassword) {
		return apperror.New(apperror.ErrInvalidCredentials, apperror.Text{
			UZ: "Joriy parol noto'g'ri", RU: "Текущий пароль неверный", EN: "The current password is incorrect",
		})
	}
	if req.NewPassword == req.CurrentPassword {
		return apperror.New(apperror.ErrInvalidData, apperror.Text{
			UZ: "Yangi parol joriy paroldan farq qilishi kerak",
			RU: "Новый пароль должен отличаться от текущего",
			EN: "The new password must differ from the current one",
		})
	}
	if err := validPassword(req.NewPassword); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hashing new password: %w", err)
	}
	rows, err := s.queries.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{
		ID: pgUUID(userID), PasswordHash: text(string(hash)), UpdatedAt: timestamp(s.now().UTC()),
	})
	if err != nil {
		return fmt.Errorf("changing password: %w", err)
	}
	if rows != 1 {
		return apperror.ErrUnauthorized
	}
	return nil
}
