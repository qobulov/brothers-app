package usecase

import "github.com/qobulov/brothers-app/internal/entities"

type UserUseCase interface {
	FindUserByID(id string) (*entities.User, error)
	FindAllUsers() ([]*entities.User, error)
	PatchUser(id string, user *entities.User) (*entities.User, error)
	DeleteUser(id string) error
}
