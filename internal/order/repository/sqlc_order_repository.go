package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/qobulov/brothers-app/internal/db"
	"github.com/qobulov/brothers-app/internal/entities"
	"github.com/qobulov/brothers-app/pkg/apperror"
)

type SQLCOrderRepository struct{ queries *db.Queries }

func NewSQLCOrderRepository(queries *db.Queries) OrderRepository {
	return &SQLCOrderRepository{queries: queries}
}

func (r *SQLCOrderRepository) Save(ctx context.Context, order *entities.Order) error {
	row, err := r.queries.CreateOrder(ctx, db.CreateOrderParams{Total: order.Total})
	if err != nil {
		return err
	}
	order.ID = uuid.UUID(row.ID.Bytes)
	return nil
}

func (r *SQLCOrderRepository) FindAll(ctx context.Context) ([]*entities.Order, error) {
	rows, err := r.queries.ListOrders(ctx)
	if err != nil {
		return nil, err
	}
	orders := make([]*entities.Order, 0, len(rows))
	for _, row := range rows {
		orders = append(orders, &entities.Order{ID: uuid.UUID(row.ID.Bytes), Total: row.Total})
	}
	return orders, nil
}

func (r *SQLCOrderRepository) FindByID(ctx context.Context, id uuid.UUID) (*entities.Order, error) {
	row, err := r.queries.GetOrderByID(ctx, db.GetOrderByIDParams{ID: orderUUIDValue(id)})
	if err != nil {
		return nil, orderError(err)
	}
	return &entities.Order{ID: uuid.UUID(row.ID.Bytes), Total: row.Total}, nil
}

func (r *SQLCOrderRepository) Patch(ctx context.Context, id uuid.UUID, order *entities.Order) (*entities.Order, error) {
	row, err := r.queries.UpdateOrder(ctx, db.UpdateOrderParams{ID: orderUUIDValue(id), Total: order.Total})
	if err != nil {
		return nil, orderError(err)
	}
	return &entities.Order{ID: uuid.UUID(row.ID.Bytes), Total: row.Total}, nil
}

func (r *SQLCOrderRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.queries.DeleteOrder(ctx, db.DeleteOrderParams{ID: orderUUIDValue(id)})
	return orderError(err)
}

func orderUUIDValue(value uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: value, Valid: true}
}

func orderError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperror.ErrRecordNotFound
	}
	return err
}
