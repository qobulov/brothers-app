package usecase

import (
	"context"
	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/internal/entities"
	"github.com/qobulov/brothers-app/internal/order/repository"
)

// OrderService
type OrderService struct {
	repo repository.OrderRepository
}

// Init OrderService function
func NewOrderService(repo repository.OrderRepository) OrderUseCase {
	return &OrderService{repo: repo}
}

// OrderService Methods - 1 create
func (s *OrderService) CreateOrder(ctx context.Context, order *entities.Order) error {
	if err := s.repo.Save(ctx, order); err != nil {
		return err
	}
	return nil
}

// OrderService Methods - 2 find all
func (s *OrderService) FindAllOrders(ctx context.Context) ([]*entities.Order, error) {
	orders, err := s.repo.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	return orders, nil
}

// OrderService Methods - 3 find by id
func (s *OrderService) FindOrderByID(ctx context.Context, id uuid.UUID) (*entities.Order, error) {
	order, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return &entities.Order{}, err
	}

	return order, nil
}

// OrderService Methods - 4 patch
func (s *OrderService) PatchOrder(ctx context.Context, id uuid.UUID, order *entities.Order) (*entities.Order, error) {
	return s.repo.Patch(ctx, id, order)
}

// OrderService Methods - 5 delete
func (s *OrderService) DeleteOrder(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	return nil
}
