package handler

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/qobulov/brothers-app/internal/entities"
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

// Register godoc
// @Summary Register a new user
// @Tags users
// @Accept json
// @Produce json
// @Param user body userdto.RegisterRequest true "User registration payload"
// @Success 201 {object} userdto.UserResponse
// @Router /auth/signup [post]
func (h *HttpUserHandler) Register(c *fiber.Ctx) error {
	req := new(userdto.RegisterRequest)
	if err := c.BodyParser(req); err != nil {
		return responses.Error(c, apperror.ErrInvalidData)
	}

	userEntity := userdto.ToUserEntity(req)
	if err := h.userUseCase.Register(userEntity); err != nil {
		return responses.Error(c, err)
	}

	return responses.Success(c, fiber.StatusCreated, userdto.ToUserResponse(userEntity), "Запрос успешно обработан")
}

// Login authenticates a legacy email/password user.
func (h *HttpUserHandler) Login(c *fiber.Ctx) error {
	loginReq := new(userdto.LoginRequest)
	if err := c.BodyParser(loginReq); err != nil {
		return responses.Error(c, apperror.ErrInvalidData)
	}

	token, userEntity, err := h.userUseCase.Login(loginReq.Email, loginReq.Password)
	if err != nil {
		return responses.ErrorWithMessage(c, apperror.ErrUnauthorized, "invalid email or password")
	}

	return responses.Success(c, fiber.StatusOK, fiber.Map{
		"user":  userdto.ToUserResponse(userEntity),
		"token": token,
	}, "Запрос успешно обработан")
}

// GetUser godoc
// @Summary Get currently authenticated user
// @Tags users
// @Produce json
// @Success 200 {object} userdto.UserResponse
// @Router /users/me [get]
func (h *HttpUserHandler) GetUser(c *fiber.Ctx) error {
	userID := c.Locals("user_id")
	if userID == nil {
		return responses.Error(c, apperror.ErrInvalidData)
	}

	userEntity, err := h.userUseCase.FindUserByID(fmt.Sprint(userID))
	if err != nil {
		return responses.Error(c, err)
	}

	return responses.Success(c, fiber.StatusOK, userdto.ToUserResponse(userEntity), "Запрос успешно обработан")
}

// FindUserByID godoc
// @Summary Get user by ID
// @Tags users
// @Produce json
// @Param id path int true "User ID"
// @Success 200 {object} userdto.UserResponse
// @Router /users/{id} [get]
func (h *HttpUserHandler) FindUserByID(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return responses.ErrorWithMessage(c, apperror.ErrInvalidData, "id is required")
	}

	userEntity, err := h.userUseCase.FindUserByID(id)
	if err != nil {
		return responses.Error(c, err)
	}

	return responses.Success(c, fiber.StatusOK, userdto.ToUserResponse(userEntity), "Запрос успешно обработан")
}

// FindAllUsers godoc
// @Summary Get all users
// @Tags users
// @Produce json
// @Success 200 {array} userdto.UserResponse
// @Router /users [get]
func (h *HttpUserHandler) FindAllUsers(c *fiber.Ctx) error {
	users, err := h.userUseCase.FindAllUsers()
	if err != nil {
		return responses.Error(c, err)
	}

	return responses.Success(c, fiber.StatusOK, userdto.ToUserResponseList(users), "Запрос успешно обработан")
}

// PatchUser godoc
// @Summary Update an user partially
// @Tags users
// @Accept json
// @Produce json
// @Param id path int true "User ID"
// @Param user body userdto.PatchUserRequest true "User update payload"
// @Success 200 {object} userdto.UserResponse
// @Router /users/{id} [patch]
func (h *HttpUserHandler) PatchUser(c *fiber.Ctx) error {
	id := c.Params("id")

	var req userdto.PatchUserRequest
	if err := c.BodyParser(&req); err != nil {
		return responses.ErrorWithMessage(c, err, "invalid request")
	}

	user := &entities.User{Name: req.Name}

	msg, err := validatePatchUser(user)
	if err != nil {
		return responses.ErrorWithMessage(c, err, msg)
	}

	updatedUser, err := h.userUseCase.PatchUser(id, user)
	if err != nil {
		return responses.Error(c, err)
	}

	return responses.Success(c, fiber.StatusOK, userdto.ToUserResponse(updatedUser), "Запрос успешно обработан")
}

// DeleteUser godoc
// @Summary Delete an user by ID
// @Tags users
// @Produce json
// @Param id path int true "User ID"
// @Success 200 {object} responses.MessageResponse
// @Router /users/{id} [delete]
func (h *HttpUserHandler) DeleteUser(c *fiber.Ctx) error {
	id := c.Params("id")

	if err := h.userUseCase.DeleteUser(id); err != nil {
		return responses.Error(c, err)
	}

	return responses.Message(c, fiber.StatusOK, "user deleted")
}

func validatePatchUser(user *entities.User) (string, error) {

	if user.Name == "" {
		return "username is invalid", apperror.ErrInvalidData
	}

	return "", nil
}
