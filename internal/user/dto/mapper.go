package userdto

import "github.com/qobulov/brothers-app/internal/entities"

// From entity.User to UserResponse
func ToUserResponse(user *entities.User) *UserResponse {
	return &UserResponse{
		ID:   user.ID,
		Name: user.Name,
	}
}

func ToUserResponseList(users []*entities.User) []*UserResponse {
	responses := make([]*UserResponse, len(users))
	for i, u := range users {
		responses[i] = ToUserResponse(u)
	}
	return responses
}
