package repository

import "github.com/qobulov/brothers-app/internal/entities"

type OrderRepository interface {
	Save(order *entities.Order) error
	FindAll() ([]*entities.Order, error)
	FindByID(id int) (*entities.Order, error)
	Patch(id int, order *entities.Order) error
	Delete(id int) error
}
