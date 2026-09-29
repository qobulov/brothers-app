package usecase

import (
	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/internal/entities"
)

type OrderUseCase interface {
	FindAllOrders() ([]*entities.Order, error)
	CreateOrder(order *entities.Order) error
	PatchOrder(id uuid.UUID, order *entities.Order) (*entities.Order, error)
	DeleteOrder(id uuid.UUID) error
	FindOrderByID(id uuid.UUID) (*entities.Order, error)
}
