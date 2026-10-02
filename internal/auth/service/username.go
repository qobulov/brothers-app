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

// NormalizeUsername trims a username and checks it against the Telegram-style
// rule: Latin letters, digits and underscores, 5 to 32 characters. The typed
// case is kept; uniqueness and lookups ignore case, so Abror and abror are the
// same username.
func NormalizeUsername(value string) (string, error) {
	username := strings.TrimSpace(value)
	if username == "" {
		return "", apperror.New(apperror.ErrInvalidData, apperror.Text{
			UZ: "Username kiritilishi shart", RU: "Укажите username", EN: "Username is required",
		})
	}
	for _, r := range username {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			return "", apperror.New(apperror.ErrInvalidData, apperror.Text{
				UZ: "Username faqat lotin harflari, raqamlar va _ belgisidan iborat bo'lishi mumkin",
				RU: "Username может содержать только латинские буквы, цифры и _",
				EN: "A username can contain only Latin letters, digits and underscores",
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
	taken, err := s.queries.UsernameTaken(ctx, db.UsernameTakenParams{Username: username})
	if err != nil {
		return dto.UsernameAvailability{}, fmt.Errorf("checking username: %w", err)
	}
	return dto.UsernameAvailability{Username: username, Available: !taken}, nil
}
