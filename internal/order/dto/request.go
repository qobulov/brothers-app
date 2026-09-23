package orderdto

type CreateOrderRequest struct {
	Total float64 `json:"total" validate:"required,gt=0"`
}
