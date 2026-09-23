package entities

type Order struct {
	ID    uint    `json:"id"`
	Total float64 `json:"total"`
}
