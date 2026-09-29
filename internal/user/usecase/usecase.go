package usecase

import (
	"context"
	"strings"

	"github.com/qobulov/brothers-app/internal/entities"
	"github.com/qobulov/brothers-app/internal/user/repository"
)

// UserService struct
type UserService struct {
	repo repository.UserRepository
}

// Init UserService
func NewUserService(repo repository.UserRepository) UserUseCase {
	return &UserService{repo: repo}
}

// SearchUsers returns only active users matching username or email.
func (s *UserService) SearchUsers(ctx context.Context, query string) ([]*entities.User, error) {
	return s.repo.Search(ctx, strings.TrimSpace(query))
}
