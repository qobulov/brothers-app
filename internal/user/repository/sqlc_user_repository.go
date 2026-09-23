package repository

import (
	"context"
	"errors"
	"time"

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

func (r *SQLCUserRepository) Save(user *entities.User) error {
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	now := time.Now().UTC()
	row, err := r.queries.CreateUser(context.Background(), db.CreateUserParams{
		ID: uuidValue(user.ID), Email: textValue(user.Email), Password: textValue(user.Password),
		Name: textValue(user.Name), Language: "uz", IsActive: true, CreatedAt: timeValue(now), UpdatedAt: timeValue(now),
	})
	if err != nil {
		return err
	}
	user.ID = uuid.UUID(row.ID.Bytes)
	return nil
}

func (r *SQLCUserRepository) FindByEmail(email string) (*entities.User, error) {
	row, err := r.queries.GetUserByEmail(context.Background(), db.GetUserByEmailParams{Email: textValue(email)})
	if err != nil {
		return nil, mapNotFound(err)
	}
	return userFromFields(row.ID, row.Email, row.Password, row.Name), nil
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
	return userFromFields(row.ID, row.Email, row.Password, row.Name), nil
}

func (r *SQLCUserRepository) FindAll() ([]*entities.User, error) {
	rows, err := r.queries.ListUsers(context.Background())
	if err != nil {
		return nil, err
	}
	users := make([]*entities.User, 0, len(rows))
	for _, row := range rows {
		users = append(users, userFromFields(row.ID, row.Email, row.Password, row.Name))
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

func userFromFields(id pgtype.UUID, email, password, name pgtype.Text) *entities.User {
	return &entities.User{ID: uuid.UUID(id.Bytes), Email: email.String, Password: password.String, Name: name.String}
}

func uuidValue(value uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: value, Valid: true} }
func textValue(value string) pgtype.Text    { return pgtype.Text{String: value, Valid: true} }
func timeValue(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func mapNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperror.ErrRecordNotFound
	}
	return err
}
