package repository

import (
	"context"

	"github.com/qobulov/brothers-app/internal/entities"
)

type UserRepository interface {
	FindByID(ctx context.Context, id string) (*entities.User, error)
	FindAll(ctx context.Context) ([]*entities.User, error)
	Search(ctx context.Context, query string) ([]*entities.User, error)
	Patch(ctx context.Context, id string, user *entities.User) (*entities.User, error)
	Delete(ctx context.Context, id string) error
}
