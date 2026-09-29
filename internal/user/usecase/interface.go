package usecase

import (
	"context"

	"github.com/qobulov/brothers-app/internal/entities"
)

type UserUseCase interface {
	FindUserByID(ctx context.Context, id string) (*entities.User, error)
	FindAllUsers(ctx context.Context) ([]*entities.User, error)
	SearchUsers(ctx context.Context, query string) ([]*entities.User, error)
	PatchUser(ctx context.Context, id string, user *entities.User) (*entities.User, error)
	DeleteUser(ctx context.Context, id string) error
}
