package userdto

type PatchUserRequest struct {
	Name string `json:"name" validate:"required"`
}
