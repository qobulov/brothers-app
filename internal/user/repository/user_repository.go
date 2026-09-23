package repository

import "github.com/qobulov/brothers-app/internal/entities"

type UserRepository interface {
	FindByID(id string) (*entities.User, error)
	FindAll() ([]*entities.User, error)
	Patch(id string, user *entities.User) error
	Delete(id string) error
}
