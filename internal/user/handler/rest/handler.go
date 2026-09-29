package handler

import (
	"strings"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
	"github.com/qobulov/brothers-app/internal/user/dto"
	"github.com/qobulov/brothers-app/internal/user/usecase"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/responses"
)

type HttpUserHandler struct {
	userUseCase usecase.UserUseCase
}

func NewHttpUserHandler(useCase usecase.UserUseCase) *HttpUserHandler {
	return &HttpUserHandler{userUseCase: useCase}
}

// Lookup godoc
// @Summary Find registered users for an invitation
// @Description Searches active users by username or email and returns only invitation-safe fields.
// @Tags users
// @Produce json
// @Param query query string true "Username or email fragment" minlength(2) maxlength(100)
// @Success 200 {array} userdto.UserResponse
// @Failure 400 {object} responses.ErrorResponse
// @Failure 401 {object} responses.ErrorResponse
// @Security BearerAuth
// @Router /users [get]
func (h *HttpUserHandler) Lookup(c *fiber.Ctx) error {
	query := strings.TrimSpace(c.Query("query"))
	if utf8.RuneCountInString(query) < 2 || utf8.RuneCountInString(query) > 100 {
		return responses.Error(c, apperror.ErrInvalidData)
	}
	users, err := h.userUseCase.SearchUsers(c.UserContext(), query)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, userdto.ToUserResponseList(users), responses.MessageUsersReturned)
}
