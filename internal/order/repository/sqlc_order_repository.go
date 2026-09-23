package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	db "github.com/qobulov/brothers-app/internal/db"
	"github.com/qobulov/brothers-app/internal/entities"
	"github.com/qobulov/brothers-app/pkg/apperror"
)

type SQLCOrderRepository struct{ queries *db.Queries }

func NewSQLCOrderRepository(queries *db.Queries) OrderRepository {
	return &SQLCOrderRepository{queries: queries}
}

func (r *SQLCOrderRepository) Save(order *entities.Order) error {
	row, err := r.queries.CreateOrder(context.Background(), db.CreateOrderParams{Total: order.Total})
	if err != nil {
		return err
	}
	order.ID = uint(row.ID)
	return nil
}

func (r *SQLCOrderRepository) FindAll() ([]*entities.Order, error) {
	rows, err := r.queries.ListOrders(context.Background())
	if err != nil {
		return nil, err
	}
	orders := make([]*entities.Order, 0, len(rows))
	for _, row := range rows {
		orders = append(orders, &entities.Order{ID: uint(row.ID), Total: row.Total})
	}
	return orders, nil
}

func (r *SQLCOrderRepository) FindByID(id int) (*entities.Order, error) {
	row, err := r.queries.GetOrderByID(context.Background(), db.GetOrderByIDParams{ID: int64(id)})
	if err != nil {
		return nil, orderError(err)
	}
	return &entities.Order{ID: uint(row.ID), Total: row.Total}, nil
}

func (r *SQLCOrderRepository) Patch(id int, order *entities.Order) error {
	_, err := r.queries.UpdateOrder(context.Background(), db.UpdateOrderParams{ID: int64(id), Total: order.Total})
	return orderError(err)
}

func (r *SQLCOrderRepository) Delete(id int) error {
	_, err := r.queries.DeleteOrder(context.Background(), db.DeleteOrderParams{ID: int64(id)})
	return orderError(err)
}

func orderError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperror.ErrRecordNotFound
	}
	return err
}
