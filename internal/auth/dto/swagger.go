package authdto

// ResponseMeta documents metadata shared by every JSON API response.
type ResponseMeta struct {
	Timestamp  string `json:"timestamp" example:"2026-09-22T12:55:03Z"`
	RequestID  string `json:"request_id" example:"aaddf89a9a081829a856613eff19682b"`
	APIVersion string `json:"api_version" example:"v1"`
	Service    string `json:"service" example:"brothers_app"`
	Duration   string `json:"duration" example:"2.22896ms"`
}

type StartResponse struct {
	Success bool         `json:"success" example:"true"`
	Code    int          `json:"code" example:"0"`
	Slug    string       `json:"slug" example:"ok"`
	Message string       `json:"message" example:"Запрос успешно обработан"`
	Data    StartData    `json:"data"`
	Meta    ResponseMeta `json:"meta"`
}

type AuthResponse struct {
	Success bool         `json:"success" example:"true"`
	Code    int          `json:"code" example:"0"`
	Slug    string       `json:"slug" example:"ok"`
	Message string       `json:"message" example:"Запрос успешно обработан"`
	Data    AuthData     `json:"data"`
	Meta    ResponseMeta `json:"meta"`
}

type UserResponse struct {
	Success bool         `json:"success" example:"true"`
	Code    int          `json:"code" example:"0"`
	Slug    string       `json:"slug" example:"ok"`
	Message string       `json:"message" example:"Запрос успешно обработан"`
	Data    UserData     `json:"data"`
	Meta    ResponseMeta `json:"meta"`
}

type ResetVerifyResponse struct {
	Success bool            `json:"success" example:"true"`
	Code    int             `json:"code" example:"0"`
	Slug    string          `json:"slug" example:"ok"`
	Message string          `json:"message" example:"Запрос успешно обработан"`
	Data    ResetVerifyData `json:"data"`
	Meta    ResponseMeta    `json:"meta"`
}

type EmptyResponse struct {
	Success bool         `json:"success" example:"true"`
	Code    int          `json:"code" example:"0"`
	Slug    string       `json:"slug" example:"ok"`
	Message string       `json:"message" example:"Запрос успешно обработан"`
	Data    any          `json:"data" extensions:"x-nullable"`
	Meta    ResponseMeta `json:"meta"`
}

type ErrorResponse struct {
	Success bool         `json:"success" example:"false"`
	Code    int          `json:"code" example:"1400"`
	Slug    string       `json:"slug" example:"invalid_data"`
	Message string       `json:"message" example:"Некорректные данные"`
	Data    any          `json:"data" extensions:"x-nullable"`
	Meta    ResponseMeta `json:"meta"`
}
