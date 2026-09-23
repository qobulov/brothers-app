package usecase

import (
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
func (s *UserService) FindUserByID(id string) (*entities.User, error) {
	return s.repo.FindByID(id)
}

// FindAllUsers returns all non-deleted users.
func (s *UserService) FindAllUsers() ([]*entities.User, error) {
	users, err := s.repo.FindAll()
	if err != nil {
		return nil, err
	}
	return users, nil
}

// PatchUser updates editable profile fields.
func (s *UserService) PatchUser(id string, user *entities.User) (*entities.User, error) {
	if err := s.repo.Patch(id, user); err != nil {
		return nil, err
	}
	updatedUser, _ := s.repo.FindByID(id)

	return updatedUser, nil
}

// DeleteUser soft-deletes a user.
func (s *UserService) DeleteUser(id string) error {
	if err := s.repo.Delete(id); err != nil {
		return err
	}
	return nil
}
