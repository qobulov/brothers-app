package repository

import (
	"context"
	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/internal/entities"
)

type OrderRepository interface {
	Save(ctx context.Context, order *entities.Order) error
	FindAll(ctx context.Context) ([]*entities.Order, error)
	FindByID(ctx context.Context, id uuid.UUID) (*entities.Order, error)
	Patch(ctx context.Context, id uuid.UUID, order *entities.Order) (*entities.Order, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
