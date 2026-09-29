package usecase

import (
	"context"
	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/internal/entities"
)

type OrderUseCase interface {
	FindAllOrders(ctx context.Context) ([]*entities.Order, error)
	CreateOrder(ctx context.Context, order *entities.Order) error
	PatchOrder(ctx context.Context, id uuid.UUID, order *entities.Order) (*entities.Order, error)
	DeleteOrder(ctx context.Context, id uuid.UUID) error
	FindOrderByID(ctx context.Context, id uuid.UUID) (*entities.Order, error)
}
