package repository

import (
	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/internal/entities"
)

type OrderRepository interface {
	Save(order *entities.Order) error
	FindAll() ([]*entities.Order, error)
	FindByID(id uuid.UUID) (*entities.Order, error)
	Patch(id uuid.UUID, order *entities.Order) error
	Delete(id uuid.UUID) error
}
