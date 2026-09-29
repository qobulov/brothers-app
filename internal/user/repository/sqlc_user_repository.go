package repository

import (
	"context"

	"github.com/google/uuid"
	db "github.com/qobulov/brothers-app/internal/db"
	"github.com/qobulov/brothers-app/internal/entities"
	"github.com/qobulov/brothers-app/pkg/helpers"
)

type SQLCUserRepository struct{ queries *db.Queries }

func NewSQLCUserRepository(queries *db.Queries) UserRepository {
	return &SQLCUserRepository{queries: queries}
}

func (r *SQLCUserRepository) Search(ctx context.Context, query string) ([]*entities.User, error) {
	rows, err := r.queries.SearchUsers(ctx, db.SearchUsersParams{Query: helpers.EscapeLike(query)})
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
