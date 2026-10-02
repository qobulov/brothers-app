package auth

import (
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/internal/auth/dto"
	"github.com/qobulov/brothers-app/internal/auth/service"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/responses"
	"strings"
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
		return responses.ErrorWithMessage(c, fmt.Errorf("%w: %w", apperror.ErrInvalidData, err), responses.MessageInvalidRequest)
	}
	data, err := h.service.Register(c.UserContext(), request)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, data, responses.MessageRequestProcessed)
}

// SendOTP godoc
// @Summary Send OTP
// @Description Sends an OTP for registration or password reset. Password reset accepts either email or username, returns 404 if the account does not exist, and returns the recipient email when delivery is started.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body authdto.SendOTPRequest true "OTP purpose and email or username"
// @Success 200 {object} authdto.StartResponse
// @Failure 400 {object} authdto.ErrorResponse
// @Failure 404 {object} authdto.ErrorResponse
// @Failure 409 {object} authdto.ErrorResponse
// @Failure 429 {object} authdto.ErrorResponse
// @Router /auth/otp/send [post]
func (h *Handler) SendOTP(c *fiber.Ctx) error {
	var request authdto.SendOTPRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.ErrorWithMessage(c, fmt.Errorf("%w: %w", apperror.ErrInvalidData, err), responses.MessageInvalidRequest)
	}
	if strings.EqualFold(strings.TrimSpace(request.Purpose), service.EmailChangePurpose) {
		return h.sendEmailChangeOTP(c, request.Email)
	}
	data, err := h.service.SendOTP(c.UserContext(), request)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, data, responses.MessageRequestProcessed)
}

// sendEmailChangeOTP needs the logged-in user, set by the optional token check on this route.
func (h *Handler) sendEmailChangeOTP(c *fiber.Ctx, newEmail string) error {
	userID, ok := c.Locals("auth_user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return responses.Error(c, apperror.ErrUnauthorized)
	}
	data, err := h.service.SendEmailChangeOTP(c.UserContext(), userID, newEmail)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, data, responses.MessageRequestProcessed)
}

// Login godoc
// @Summary Sign in with username or email
// @Description Authenticates an active user using a username or email address and password.
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
		return responses.ErrorWithMessage(c, fmt.Errorf("%w: %w", apperror.ErrInvalidData, err), responses.MessageInvalidRequest)
	}
	data, err := h.service.Login(c.UserContext(), request.Login, request.Password)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, data, responses.MessageRequestProcessed)
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
		return responses.ErrorWithMessage(c, fmt.Errorf("%w: %w", apperror.ErrInvalidData, err), responses.MessageInvalidRequest)
	}
	data, err := h.service.Refresh(c.UserContext(), request.RefreshToken)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, data, responses.MessageRequestProcessed)
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
	return responses.Success(c, fiber.StatusOK, data, responses.MessageRequestProcessed)
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
		return responses.ErrorWithMessage(c, fmt.Errorf("%w: %w", apperror.ErrInvalidData, err), responses.MessageInvalidRequest)
	}
	data, err := h.service.UpdateCurrentUser(c.UserContext(), userID, request)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, data, responses.MessageRequestProcessed)
}

// VerifyPassword godoc
// @Summary Verify password-reset OTP
// @Description Exchanges an email-delivered reset OTP for a single-use reset token.
// @Tags auth
// @Accept json
// @Produce json
// @Param verification body authdto.OTPVerifyRequest true "Email and OTP"
// @Success 200 {object} authdto.ResetVerifyResponse
// @Failure 400 {object} authdto.ErrorResponse
// @Failure 429 {object} authdto.ErrorResponse
// @Router /auth/password/verify [post]
func (h *Handler) VerifyPassword(c *fiber.Ctx) error {
	var request authdto.OTPVerifyRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.ErrorWithMessage(c, fmt.Errorf("%w: %w", apperror.ErrInvalidData, err), responses.MessageInvalidRequest)
	}
	data, err := h.service.VerifyPasswordOTP(c.UserContext(), request.Email, request.OTP)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, data, responses.MessageRequestProcessed)
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
		return responses.ErrorWithMessage(c, fmt.Errorf("%w: %w", apperror.ErrInvalidData, err), responses.MessageInvalidRequest)
	}
	if err := h.service.ResetPassword(c.UserContext(), request); err != nil {
		return responses.Error(c, err)
	}
	return responses.Success[any](c, fiber.StatusOK, nil, responses.MessageRequestProcessed)
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
	return responses.Success[any](c, fiber.StatusOK, nil, responses.MessageRequestProcessed)
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

// CheckUsername godoc
// @Summary Check whether a username is available
// @Description Public, no token. Usernames are Latin letters, digits and underscores, 5-32 characters. The typed case is kept, but uniqueness ignores case (Abror and abror are the same). The response returns the trimmed username. A taken username returns 200 with available=false; a badly formatted one returns 400.
// @Tags auth
// @Produce json
// @Param username query string true "Username to check" minlength(5) maxlength(32)
// @Success 200 {object} authdto.UsernameAvailabilityResponse
// @Failure 400 {object} authdto.ErrorResponse
// @Router /auth/username/check [get]
func (h *Handler) CheckUsername(c *fiber.Ctx) error {
	data, err := h.service.CheckUsername(c.UserContext(), c.Query("username"))
	if err != nil {
		return responses.Error(c, err)
	}
	message := responses.MessageUsernameAvailable
	if !data.Available {
		message = responses.MessageUsernameTaken
	}
	return responses.Success(c, fiber.StatusOK, data, message)
}

// ChangeEmail godoc
// @Summary Change my email
// @Description First request a code with POST /auth/otp/send, purpose email_change, the new address as email and this token. Then confirm here with the code sent to the new address. The current session stays valid.
// @Tags profile
// @Accept json
// @Produce json
// @Param request body authdto.ChangeEmailRequest true "New email and OTP"
// @Success 200 {object} authdto.UserResponse
// @Failure 400 {object} authdto.ErrorResponse
// @Failure 401 {object} authdto.ErrorResponse
// @Failure 409 {object} authdto.ErrorResponse
// @Security BearerAuth
// @Router /me/email [post]
func (h *Handler) ChangeEmail(c *fiber.Ctx) error {
	userID, ok := c.Locals("auth_user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return responses.Error(c, apperror.ErrUnauthorized)
	}
	var request authdto.ChangeEmailRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.ErrorWithMessage(c, fmt.Errorf("%w: %w", apperror.ErrInvalidData, err), responses.MessageInvalidRequest)
	}
	user, err := h.service.ChangeEmail(c.UserContext(), userID, request)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, user, responses.MessageEmailChanged)
}
