// Package user looks up registered accounts, for example to invite them to a group.
package user

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	db "github.com/qobulov/brothers-app/internal/db"
	"github.com/qobulov/brothers-app/pkg/helpers"
)

// User holds only the fields that are safe to show to other users.
type User struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	AvatarURL string    `json:"avatar_url"`
}

type Service struct {
	queries *db.Queries
}

func NewService(queries *db.Queries) *Service {
	return &Service{queries: queries}
}

// Search returns active users whose username or email contains the query.
func (s *Service) Search(ctx context.Context, query string) ([]User, error) {
	pattern := helpers.EscapeLike(strings.TrimSpace(query))
	rows, err := s.queries.SearchUsers(ctx, db.SearchUsersParams{Query: pattern})
	if err != nil {
		return nil, fmt.Errorf("searching users: %w", err)
	}
	users := make([]User, 0, len(rows))
	for _, row := range rows {
		users = append(users, User{
			ID:        uuid.UUID(row.ID.Bytes),
			Username:  row.Username.String,
			Email:     row.Email.String,
			AvatarURL: row.AvatarUrl.String,
		})
	}
	return users, nil
}
