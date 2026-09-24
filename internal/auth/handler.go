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

// Register godoc
// @Summary Register user
// @Description Verifies the registration OTP, creates the user, and issues a token pair. Request an OTP first through POST /auth/otp/send.
// @Tags auth
// @Accept json
// @Produce json
// @Param user body authdto.RegisterRequest true "Registration payload"
// @Success 200 {object} authdto.RegisterResponse
// @Failure 400 {object} authdto.ErrorResponse
// @Failure 409 {object} authdto.ErrorResponse
// @Router /auth/register [post]
func (h *Handler) Register(c *fiber.Ctx) error {
	var request authdto.RegisterRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.Error(c, apperror.ErrInvalidData)
	}
	if request.Language == "" {
		request.Language = "uz"
	}
	data, err := h.service.Register(c.UserContext(), request)
	if err != nil {
		return responses.ErrorLocalized(c, err, request.Language)
	}
	return responses.Success(c, fiber.StatusOK, data, "Request processed successfully")
}

// SendOTP godoc
// @Summary Send OTP
// @Description Starts Telegram OTP delivery for registration or password reset. Use purpose registration or password_reset, then open the returned deep link and press Start.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body authdto.SendOTPRequest true "Phone and OTP purpose"
// @Success 200 {object} authdto.StartResponse
// @Failure 400 {object} authdto.ErrorResponse
// @Failure 409 {object} authdto.ErrorResponse
// @Failure 429 {object} authdto.ErrorResponse
// @Router /auth/otp/send [post]
func (h *Handler) SendOTP(c *fiber.Ctx) error {
	var request authdto.SendOTPRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.Error(c, apperror.ErrInvalidData)
	}
	data, err := h.service.SendOTP(c.UserContext(), request)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, data, "Request processed successfully")
}

// Login godoc
// @Summary Sign in with username or phone
// @Description Authenticates an active user using a username or phone number and password.
// @Tags auth
// @Accept json
// @Produce json
// @Param credentials body authdto.LoginRequest true "Login credentials"
// @Success 200 {object} authdto.AuthResponse
// @Failure 400 {object} authdto.ErrorResponse
// @Failure 401 {object} authdto.ErrorResponse
// @Failure 429 {object} authdto.ErrorResponse
// @Router /auth/login [post]
func (h *Handler) Login(c *fiber.Ctx) error {
	var request authdto.LoginRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.Error(c, apperror.ErrInvalidData)
	}
	data, err := h.service.Login(c.UserContext(), request.Login, request.Password)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, data, "Запрос успешно обработан")
}

// Refresh godoc
// @Summary Refresh session tokens
// @Description Rotates the opaque refresh token and returns a new token pair.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body authdto.RefreshRequest true "Refresh token"
// @Success 200 {object} authdto.AuthResponse
// @Failure 400 {object} authdto.ErrorResponse
// @Failure 401 {object} authdto.ErrorResponse
// @Failure 429 {object} authdto.ErrorResponse
// @Router /auth/refresh [post]
func (h *Handler) Refresh(c *fiber.Ctx) error {
	var request authdto.RefreshRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.Error(c, apperror.ErrInvalidData)
	}
	data, err := h.service.Refresh(c.UserContext(), request.RefreshToken)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, data, "Запрос успешно обработан")
}

// CurrentUser godoc
// @Summary Get current profile
// @Description Returns safe profile fields for the authenticated user.
// @Tags profile
// @Produce json
// @Success 200 {object} authdto.UserResponse
// @Failure 401 {object} authdto.ErrorResponse
// @Security BearerAuth
// @Router /me [get]
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

// UpdateCurrentUser godoc
// @Summary Update current profile
// @Description Updates only provided names, avatar URL, and language fields.
// @Tags profile
// @Accept json
// @Produce json
// @Param profile body authdto.UpdateProfileRequest true "Editable profile fields"
// @Success 200 {object} authdto.UserResponse
// @Failure 400 {object} authdto.ErrorResponse
// @Failure 401 {object} authdto.ErrorResponse
// @Security BearerAuth
// @Router /me [patch]
func (h *Handler) UpdateCurrentUser(c *fiber.Ctx) error {
	userID, _, err := authLocals(c)
	if err != nil {
		return responses.Error(c, err)
	}
	var request authdto.UpdateProfileRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.Error(c, apperror.ErrInvalidData)
	}
	data, err := h.service.UpdateCurrentUser(c.UserContext(), userID, request)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, data, "Запрос успешно обработан")
}

// VerifyPassword godoc
// @Summary Verify password-reset OTP
// @Description Exchanges a Telegram-delivered reset OTP for a single-use reset token.
// @Tags auth
// @Accept json
// @Produce json
// @Param verification body authdto.OTPVerifyRequest true "Phone and OTP"
// @Success 200 {object} authdto.ResetVerifyResponse
// @Failure 400 {object} authdto.ErrorResponse
// @Failure 429 {object} authdto.ErrorResponse
// @Router /auth/password/verify [post]
func (h *Handler) VerifyPassword(c *fiber.Ctx) error {
	var request authdto.OTPVerifyRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.Error(c, apperror.ErrInvalidData)
	}
	data, err := h.service.VerifyPasswordOTP(c.UserContext(), request.Phone, request.OTP)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, data, "Запрос успешно обработан")
}

// ResetPassword godoc
// @Summary Reset password
// @Description Replaces the password using a reset-only token and revokes the active session.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body authdto.ResetPasswordRequest true "Reset token and new password"
// @Success 200 {object} authdto.EmptyResponse
// @Failure 400 {object} authdto.ErrorResponse
// @Failure 401 {object} authdto.ErrorResponse
// @Router /auth/password/reset [post]
func (h *Handler) ResetPassword(c *fiber.Ctx) error {
	var request authdto.ResetPasswordRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.Error(c, apperror.ErrInvalidData)
	}
	if err := h.service.ResetPassword(c.UserContext(), request); err != nil {
		return responses.Error(c, err)
	}
	return responses.Success[any](c, fiber.StatusOK, nil, "Запрос успешно обработан")
}

// Logout godoc
// @Summary Log out
// @Description Revokes the current authenticated session.
// @Tags auth
// @Produce json
// @Success 200 {object} authdto.EmptyResponse
// @Failure 401 {object} authdto.ErrorResponse
// @Security BearerAuth
// @Router /auth/logout [post]
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

// PhoneChangeRequest godoc
// @Summary Start phone change
// @Description Starts the Telegram OTP flow for a new unique phone number.
// @Tags profile
// @Accept json
// @Produce json
// @Param request body authdto.PhoneChangeRequest true "New phone"
// @Success 200 {object} authdto.StartResponse
// @Failure 400 {object} authdto.ErrorResponse
// @Failure 401 {object} authdto.ErrorResponse
// @Failure 409 {object} authdto.ErrorResponse
// @Security BearerAuth
// @Router /me/phone-change/request [post]
func (h *Handler) PhoneChangeRequest(c *fiber.Ctx) error {
	userID, _, err := authLocals(c)
	if err != nil {
		return responses.Error(c, err)
	}
	var request authdto.PhoneChangeRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.Error(c, apperror.ErrInvalidData)
	}
	data, err := h.service.PhoneChangeRequest(c.UserContext(), userID, request.NewPhone)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, data, "Запрос успешно обработан")
}

// PhoneChangeConfirm godoc
// @Summary Confirm phone change
// @Description Verifies the Telegram OTP and applies the new phone number.
// @Tags profile
// @Accept json
// @Produce json
// @Param confirmation body authdto.PhoneChangeConfirmRequest true "New phone and OTP"
// @Success 200 {object} authdto.EmptyResponse
// @Failure 400 {object} authdto.ErrorResponse
// @Failure 401 {object} authdto.ErrorResponse
// @Failure 409 {object} authdto.ErrorResponse
// @Security BearerAuth
// @Router /me/phone-change/confirm [post]
func (h *Handler) PhoneChangeConfirm(c *fiber.Ctx) error {
	userID, _, err := authLocals(c)
	if err != nil {
		return responses.Error(c, err)
	}
	var request authdto.PhoneChangeConfirmRequest
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
