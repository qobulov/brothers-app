package repository

import (
	"context"

	"github.com/qobulov/brothers-app/internal/entities"
)

type UserRepository interface {
	Search(ctx context.Context, query string) ([]*entities.User, error)
}
