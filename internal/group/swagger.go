package group

import "github.com/qobulov/brothers-app/pkg/responses"

// GroupResponse documents the standard success envelope for one group.
type GroupResponse struct {
	Success bool           `json:"success" example:"true"`
	Code    int            `json:"code" example:"0"`
	Slug    string         `json:"slug" example:"ok"`
	Message string         `json:"message" example:"group created"`
	Data    Group          `json:"data"`
	Meta    responses.Meta `json:"meta"`
}

// GroupsResponse documents the standard success envelope for group lists.
type GroupsResponse struct {
	Success bool            `json:"success" example:"true"`
	Code    int             `json:"code" example:"0"`
	Slug    string          `json:"slug" example:"ok"`
	Message string          `json:"message" example:"groups returned"`
	Data    []GroupListItem `json:"data"`
	Meta    responses.Meta  `json:"meta"`
}

// InvitationResponse documents the standard success envelope for one invitation.
type InvitationResponse struct {
	Success bool           `json:"success" example:"true"`
	Code    int            `json:"code" example:"0"`
	Slug    string         `json:"slug" example:"ok"`
	Message string         `json:"message" example:"invitation created"`
	Data    Invitation     `json:"data"`
	Meta    responses.Meta `json:"meta"`
}

// InvitationsResponse documents the standard success envelope for invitation lists.
type InvitationsResponse struct {
	Success bool           `json:"success" example:"true"`
	Code    int            `json:"code" example:"0"`
	Slug    string         `json:"slug" example:"ok"`
	Message string         `json:"message" example:"invitations returned"`
	Data    []Invitation   `json:"data"`
	Meta    responses.Meta `json:"meta"`
}

type MemberResponse struct {
	Success bool           `json:"success" example:"true"`
	Code    int            `json:"code" example:"0"`
	Slug    string         `json:"slug" example:"ok"`
	Message string         `json:"message" example:"member invitation created"`
	Data    Member         `json:"data"`
	Meta    responses.Meta `json:"meta"`
}

type MembersResponse struct {
	Success bool           `json:"success" example:"true"`
	Code    int            `json:"code" example:"0"`
	Slug    string         `json:"slug" example:"ok"`
	Message string         `json:"message" example:"members returned"`
	Data    []Member       `json:"data"`
	Meta    responses.Meta `json:"meta"`
}

type LocationResponse struct {
	Success bool           `json:"success" example:"true"`
	Code    int            `json:"code" example:"0"`
	Slug    string         `json:"slug" example:"ok"`
	Message string         `json:"message" example:"request processed successfully"`
	Data    Location       `json:"data"`
	Meta    responses.Meta `json:"meta"`
}

type LocationsResponse struct {
	Success bool           `json:"success" example:"true"`
	Code    int            `json:"code" example:"0"`
	Slug    string         `json:"slug" example:"ok"`
	Message string         `json:"message" example:"request processed successfully"`
	Data    []Location     `json:"data"`
	Meta    responses.Meta `json:"meta"`
}

type CustomersResponse struct {
	Success bool           `json:"success" example:"true"`
	Code    int            `json:"code" example:"0"`
	Slug    string         `json:"slug" example:"ok"`
	Message string         `json:"message" example:"request processed successfully"`
	Data    []Customer     `json:"data"`
	Meta    responses.Meta `json:"meta"`
}

// ErrorResponse documents standard API failures.
type ErrorResponse struct {
	Success bool                    `json:"success" example:"false"`
	Code    int                     `json:"code" example:"1400"`
	Slug    string                  `json:"slug" example:"invalid_data"`
	Message string                  `json:"message" example:"Invalid request"`
	Data    *responses.ErrorDetails `json:"data"`
	Meta    responses.Meta          `json:"meta"`
}

// MemberDetailResponse documents the standard success envelope for one member.
type MemberDetailResponse struct {
	Success bool           `json:"success" example:"true"`
	Code    int            `json:"code" example:"0"`
	Slug    string         `json:"slug" example:"ok"`
	Message string         `json:"message" example:"Request processed successfully"`
	Data    MemberDetail   `json:"data"`
	Meta    responses.Meta `json:"meta"`
}

// BalanceAdjustmentResponse documents the standard success envelope for one balance adjustment.
type BalanceAdjustmentResponse struct {
	Success bool              `json:"success" example:"true"`
	Code    int               `json:"code" example:"0"`
	Slug    string            `json:"slug" example:"ok"`
	Message string            `json:"message" example:"Balance updated"`
	Data    BalanceAdjustment `json:"data"`
	Meta    responses.Meta    `json:"meta"`
}

// BalanceHistoryResponse documents the standard success envelope for a member's balance history.
type BalanceHistoryResponse struct {
	Success bool           `json:"success" example:"true"`
	Code    int            `json:"code" example:"0"`
	Slug    string         `json:"slug" example:"ok"`
	Message string         `json:"message" example:"Request processed successfully"`
	Data    BalanceHistory `json:"data"`
	Meta    responses.Meta `json:"meta"`
}
