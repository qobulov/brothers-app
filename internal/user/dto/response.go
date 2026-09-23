package userdto

import "github.com/google/uuid"

type UserResponse struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}
