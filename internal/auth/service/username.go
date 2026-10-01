package service

import (
	"context"
	"fmt"
	"strings"

	dto "github.com/qobulov/brothers-app/internal/auth/dto"
	db "github.com/qobulov/brothers-app/internal/db"
	"github.com/qobulov/brothers-app/pkg/apperror"
)

const (
	minUsernameLength = 5
	maxUsernameLength = 32
)

// NormalizeUsername trims and lowercases a username and checks it against the
// Telegram-style rule: a-z, 0-9 and underscores, 5 to 32 characters. Storing
// usernames in lowercase makes uniqueness case-insensitive.
func NormalizeUsername(value string) (string, error) {
	username := strings.ToLower(strings.TrimSpace(value))
	if username == "" {
		return "", apperror.New(apperror.ErrInvalidData, apperror.Text{
			UZ: "Username kiritilishi shart", RU: "Укажите username", EN: "Username is required",
		})
	}
	for _, r := range username {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return "", apperror.New(apperror.ErrInvalidData, apperror.Text{
				UZ: "Username faqat a-z, 0-9 va _ belgilaridan iborat bo'lishi mumkin",
				RU: "Username может содержать только a-z, 0-9 и _",
				EN: "A username can contain only a-z, 0-9 and underscores",
			})
		}
	}
	if len(username) < minUsernameLength || len(username) > maxUsernameLength {
		return "", apperror.New(apperror.ErrInvalidData, apperror.Text{
			UZ: fmt.Sprintf("Username %d dan %d belgigacha bo'lishi kerak", minUsernameLength, maxUsernameLength),
			RU: fmt.Sprintf("Username должен содержать от %d до %d символов", minUsernameLength, maxUsernameLength),
			EN: fmt.Sprintf("A username must be %d-%d characters", minUsernameLength, maxUsernameLength),
		})
	}
	return username, nil
}

// CheckUsername reports whether a username is free to register. Deleted
// accounts release their username.
func (s *Service) CheckUsername(ctx context.Context, value string) (dto.UsernameAvailability, error) {
	username, err := NormalizeUsername(value)
	if err != nil {
		return dto.UsernameAvailability{}, err
	}
	taken, err := s.queries.UsernameTaken(ctx, db.UsernameTakenParams{Username: text(username)})
	if err != nil {
		return dto.UsernameAvailability{}, fmt.Errorf("checking username: %w", err)
	}
	return dto.UsernameAvailability{Username: username, Available: !taken}, nil
}
