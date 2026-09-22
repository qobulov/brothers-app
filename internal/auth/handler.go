package auth

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/internal/auth/dto"
	"github.com/qobulov/brothers-app/internal/auth/service"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/responses"
)

type Handler struct{ service *service.Service }

func NewHandler(authService *service.Service) *Handler { return &Handler{service: authService} }

func (h *Handler) Register(c *fiber.Ctx) error {
	var request dto.RegisterRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.Error(c, apperror.ErrInvalidData)
	}
	data, err := h.service.Register(c.UserContext(), request)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusCreated, data, "Запрос успешно обработан")
}

func (h *Handler) Login(c *fiber.Ctx) error {
	var request dto.LoginRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.Error(c, apperror.ErrInvalidData)
	}
	data, err := h.service.Login(c.UserContext(), request.Identifier, request.Password)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, data, "Запрос успешно обработан")
}

func (h *Handler) Refresh(c *fiber.Ctx) error {
	var request struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.BodyParser(&request); err != nil {
		return responses.Error(c, apperror.ErrInvalidData)
	}
	data, err := h.service.Refresh(c.UserContext(), request.RefreshToken)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, data, "Запрос успешно обработан")
}

func (h *Handler) CurrentUser(c *fiber.Ctx) error {
	userID, _, err := authLocals(c)
	if err != nil {
		return responses.Error(c, err)
	}
	data, err := h.service.CurrentUser(c.UserContext(), userID)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, data, "Запрос успешно обработан")
}

func (h *Handler) VerifyRegistration(c *fiber.Ctx) error {
	var request dto.OTPRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.Error(c, apperror.ErrInvalidData)
	}
	data, err := h.service.VerifyRegistration(c.UserContext(), request.Phone, request.OTP)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, data, "Запрос успешно обработан")
}

func (h *Handler) ResendRegistration(c *fiber.Ctx) error {
	var request dto.OTPRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.Error(c, apperror.ErrInvalidData)
	}
	if err := h.service.Resend(c.UserContext(), "registration", request.Phone, nil); err != nil {
		return responses.Error(c, err)
	}
	return responses.Success[any](c, fiber.StatusOK, nil, "Запрос успешно обработан")
}

func (h *Handler) ForgotPassword(c *fiber.Ctx) error {
	var request dto.ForgotPasswordRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.Error(c, apperror.ErrInvalidData)
	}
	data, err := h.service.ForgotPassword(c.UserContext(), request.Phone)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, data, "Запрос успешно обработан")
}

func (h *Handler) ResendPassword(c *fiber.Ctx) error {
	var request dto.OTPRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.Error(c, apperror.ErrInvalidData)
	}
	if err := h.service.Resend(c.UserContext(), "password_reset", request.Phone, nil); err != nil {
		return responses.Error(c, err)
	}
	return responses.Success[any](c, fiber.StatusOK, nil, "Запрос успешно обработан")
}

func (h *Handler) VerifyPassword(c *fiber.Ctx) error {
	var request dto.OTPRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.Error(c, apperror.ErrInvalidData)
	}
	data, err := h.service.VerifyPasswordOTP(c.UserContext(), request.Phone, request.OTP)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, data, "Запрос успешно обработан")
}

func (h *Handler) ResetPassword(c *fiber.Ctx) error {
	var request dto.ResetPasswordRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.Error(c, apperror.ErrInvalidData)
	}
	if err := h.service.ResetPassword(c.UserContext(), request); err != nil {
		return responses.Error(c, err)
	}
	return responses.Success[any](c, fiber.StatusOK, nil, "Запрос успешно обработан")
}

func (h *Handler) Logout(c *fiber.Ctx) error {
	userID, sessionID, err := authLocals(c)
	if err != nil {
		return responses.Error(c, err)
	}
	if err := h.service.Logout(c.UserContext(), userID, sessionID); err != nil {
		return responses.Error(c, err)
	}
	return responses.Success[any](c, fiber.StatusOK, nil, "Запрос успешно обработан")
}

func (h *Handler) PhoneChangeRequest(c *fiber.Ctx) error {
	userID, _, err := authLocals(c)
	if err != nil {
		return responses.Error(c, err)
	}
	var request dto.PhoneChangeRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.Error(c, apperror.ErrInvalidData)
	}
	data, err := h.service.PhoneChangeRequest(c.UserContext(), userID, request.NewPhone)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, data, "Запрос успешно обработан")
}

func (h *Handler) PhoneChangeResend(c *fiber.Ctx) error {
	userID, _, err := authLocals(c)
	if err != nil {
		return responses.Error(c, err)
	}
	var request dto.PhoneChangeRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.Error(c, apperror.ErrInvalidData)
	}
	if err := h.service.Resend(c.UserContext(), "phone_change", request.NewPhone, &userID); err != nil {
		return responses.Error(c, err)
	}
	return responses.Success[any](c, fiber.StatusOK, nil, "Запрос успешно обработан")
}

func (h *Handler) PhoneChangeConfirm(c *fiber.Ctx) error {
	userID, _, err := authLocals(c)
	if err != nil {
		return responses.Error(c, err)
	}
	var request dto.PhoneChangeConfirmRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.Error(c, apperror.ErrInvalidData)
	}
	if err := h.service.PhoneChangeConfirm(c.UserContext(), userID, request.NewPhone, request.OTP); err != nil {
		return responses.Error(c, err)
	}
	return responses.Success[any](c, fiber.StatusOK, nil, "Запрос успешно обработан")
}

func authLocals(c *fiber.Ctx) (uuid.UUID, uuid.UUID, error) {
	userID, ok := c.Locals("auth_user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return uuid.Nil, uuid.Nil, apperror.ErrUnauthorized
	}
	sessionID, ok := c.Locals("auth_session_id").(uuid.UUID)
	if !ok || sessionID == uuid.Nil {
		return uuid.Nil, uuid.Nil, apperror.ErrUnauthorized
	}
	return userID, sessionID, nil
}
