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

type SQLCUserRepository struct{ queries *db.Queries }

func NewSQLCUserRepository(queries *db.Queries) UserRepository {
	return &SQLCUserRepository{queries: queries}
}

func (r *SQLCUserRepository) FindByID(ctx context.Context, id string) (*entities.User, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, apperror.ErrInvalidID
	}
	row, err := r.queries.GetUserByID(ctx, db.GetUserByIDParams{ID: uuidValue(parsed)})
	if err != nil {
		return nil, mapNotFound(err)
	}
	return userFromModel(row), nil
}

func (r *SQLCUserRepository) FindAll(ctx context.Context) ([]*entities.User, error) {
	rows, err := r.queries.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	users := make([]*entities.User, 0, len(rows))
	for _, row := range rows {
		users = append(users, userFromModel(row))
	}
	return users, nil
}

func (r *SQLCUserRepository) Search(ctx context.Context, query string) ([]*entities.User, error) {
	rows, err := r.queries.SearchUsers(ctx, db.SearchUsersParams{Query: query})
	if err != nil {
		return nil, err
	}
	users := make([]*entities.User, 0, len(rows))
	for _, row := range rows {
		users = append(users, &entities.User{
			ID:        uuid.UUID(row.ID.Bytes),
			Username:  row.Username.String,
			Email:     row.Email.String,
			AvatarURL: row.AvatarUrl.String,
		})
	}
	return users, nil
}

func (r *SQLCUserRepository) Patch(ctx context.Context, id string, user *entities.User) (*entities.User, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, apperror.ErrInvalidID
	}
	row, err := r.queries.UpdateUserName(ctx, db.UpdateUserNameParams{ID: uuidValue(parsed), Name: textValue(user.Name)})
	if err != nil {
		return nil, mapNotFound(err)
	}
	return userFromModel(row), nil
}

func (r *SQLCUserRepository) Delete(ctx context.Context, id string) error {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return apperror.ErrInvalidID
	}
	_, err = r.queries.SoftDeleteUser(ctx, db.SoftDeleteUserParams{ID: uuidValue(parsed)})
	return mapNotFound(err)
}

func userFromModel(user db.User) *entities.User {
	result := &entities.User{
		ID:           uuid.UUID(user.ID.Bytes),
		Password:     user.Password.String,
		PasswordHash: user.PasswordHash.String,
		Name:         user.Name.String,
		Email:        user.Email.String,
		Phone:        user.Phone.String,
		Username:     user.Username.String,
		FirstName:    user.FirstName.String,
		LastName:     user.LastName.String,
		AvatarURL:    user.AvatarUrl.String,
		Language:     user.Language,
		IsActive:     user.IsActive,
		CreatedAt:    user.CreatedAt.Time,
		UpdatedAt:    user.UpdatedAt.Time,
	}
	if user.LastLoginAt.Valid {
		result.LastLoginAt = &user.LastLoginAt.Time
	}
	if user.DeletedAt.Valid {
		result.DeletedAt = &user.DeletedAt.Time
	}
	return result
}

func uuidValue(value uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: value, Valid: true} }
func textValue(value string) pgtype.Text    { return pgtype.Text{String: value, Valid: true} }
func mapNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperror.ErrRecordNotFound
	}
	return err
}
