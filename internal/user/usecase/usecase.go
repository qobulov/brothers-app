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

// FindUserByID returns a user by identifier.
func (s *UserService) FindUserByID(ctx context.Context, id string) (*entities.User, error) {
	return s.repo.FindByID(ctx, id)
}

// FindAllUsers returns all non-deleted users.
func (s *UserService) FindAllUsers(ctx context.Context) ([]*entities.User, error) {
	users, err := s.repo.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	return users, nil
}

// SearchUsers returns only active users matching username or email.
func (s *UserService) SearchUsers(ctx context.Context, query string) ([]*entities.User, error) {
	return s.repo.Search(ctx, strings.TrimSpace(query))
}

// PatchUser updates editable profile fields.
func (s *UserService) PatchUser(ctx context.Context, id string, user *entities.User) (*entities.User, error) {
	return s.repo.Patch(ctx, id, user)
}

// DeleteUser soft-deletes a user.
func (s *UserService) DeleteUser(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	return nil
}
