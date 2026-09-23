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

func (r *SQLCUserRepository) FindByID(id string) (*entities.User, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, apperror.ErrInvalidID
	}
	row, err := r.queries.GetUserByID(context.Background(), db.GetUserByIDParams{ID: uuidValue(parsed)})
	if err != nil {
		return nil, mapNotFound(err)
	}
	return userFromModel(row), nil
}

func (r *SQLCUserRepository) FindAll() ([]*entities.User, error) {
	rows, err := r.queries.ListUsers(context.Background())
	if err != nil {
		return nil, err
	}
	users := make([]*entities.User, 0, len(rows))
	for _, row := range rows {
		users = append(users, userFromModel(row))
	}
	return users, nil
}

func (r *SQLCUserRepository) Patch(id string, user *entities.User) error {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return apperror.ErrInvalidID
	}
	_, err = r.queries.UpdateUserName(context.Background(), db.UpdateUserNameParams{ID: uuidValue(parsed), Name: textValue(user.Name)})
	return mapNotFound(err)
}

func (r *SQLCUserRepository) Delete(id string) error {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return apperror.ErrInvalidID
	}
	_, err = r.queries.SoftDeleteUser(context.Background(), db.SoftDeleteUserParams{ID: uuidValue(parsed)})
	return mapNotFound(err)
}

func userFromModel(user db.User) *entities.User {
	result := &entities.User{
		ID:           uuid.UUID(user.ID.Bytes),
		Password:     user.Password.String,
		PasswordHash: user.PasswordHash.String,
		Name:         user.Name.String,
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
