package usecase

import (
	"context"

	"github.com/qobulov/brothers-app/internal/entities"
)

type UserUseCase interface {
	FindUserByID(id string) (*entities.User, error)
	FindAllUsers() ([]*entities.User, error)
	SearchUsers(ctx context.Context, query string) ([]*entities.User, error)
	PatchUser(id string, user *entities.User) (*entities.User, error)
	DeleteUser(id string) error
}
