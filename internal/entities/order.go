package entities

import "github.com/google/uuid"

type Order struct {
	ID    uuid.UUID `json:"id"`
	Total float64   `json:"total"`
}
