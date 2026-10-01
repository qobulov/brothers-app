package debt

import "github.com/qobulov/brothers-app/pkg/responses"

// DebtResponse documents the standard success envelope for one debt.
type DebtResponse struct {
	Success bool           `json:"success" example:"true"`
	Code    int            `json:"code" example:"0"`
	Slug    string         `json:"slug" example:"ok"`
	Message string         `json:"message" example:"Debt created"`
	Data    Debt           `json:"data"`
	Meta    responses.Meta `json:"meta"`
}

// DebtsResponse documents the standard success envelope for a debt list.
type DebtsResponse struct {
	Success bool           `json:"success" example:"true"`
	Code    int            `json:"code" example:"0"`
	Slug    string         `json:"slug" example:"ok"`
	Message string         `json:"message" example:"Request processed successfully"`
	Data    []Debt         `json:"data"`
	Meta    responses.Meta `json:"meta"`
}

// SummaryResponse documents the standard success envelope for debt totals.
type SummaryResponse struct {
	Success bool           `json:"success" example:"true"`
	Code    int            `json:"code" example:"0"`
	Slug    string         `json:"slug" example:"ok"`
	Message string         `json:"message" example:"Request processed successfully"`
	Data    Summary        `json:"data"`
	Meta    responses.Meta `json:"meta"`
}

// RepaymentsResponse documents the standard success envelope for repayment history.
type RepaymentsResponse struct {
	Success bool           `json:"success" example:"true"`
	Code    int            `json:"code" example:"0"`
	Slug    string         `json:"slug" example:"ok"`
	Message string         `json:"message" example:"Request processed successfully"`
	Data    []Repayment    `json:"data"`
	Meta    responses.Meta `json:"meta"`
}
