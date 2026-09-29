package usecase

import (
	"context"

	"github.com/qobulov/brothers-app/internal/entities"
)

type UserUseCase interface {
	SearchUsers(ctx context.Context, query string) ([]*entities.User, error)
}
