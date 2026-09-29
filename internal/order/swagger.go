package order

import "github.com/qobulov/brothers-app/pkg/responses"

// OrderResponse documents the standard success envelope for one order.
type OrderResponse struct {
	Success bool           `json:"success" example:"true"`
	Code    int            `json:"code" example:"0"`
	Slug    string         `json:"slug" example:"ok"`
	Message string         `json:"message" example:"Order created"`
	Data    Order          `json:"data"`
	Meta    responses.Meta `json:"meta"`
}

// OrdersResponse documents the standard success envelope for order lists.
type OrdersResponse struct {
	Success bool           `json:"success" example:"true"`
	Code    int            `json:"code" example:"0"`
	Slug    string         `json:"slug" example:"ok"`
	Message string         `json:"message" example:"Orders returned"`
	Data    []ListItem     `json:"data"`
	Meta    responses.Meta `json:"meta"`
}

// OrderEventsResponse documents the standard success envelope for order history.
type OrderEventsResponse struct {
	Success bool           `json:"success" example:"true"`
	Code    int            `json:"code" example:"0"`
	Slug    string         `json:"slug" example:"ok"`
	Message string         `json:"message" example:"Request processed successfully"`
	Data    []Event        `json:"data"`
	Meta    responses.Meta `json:"meta"`
}
