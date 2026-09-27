package group

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/responses"
)

type Handler struct {
	service *Service
}

type CreateRequest struct {
	Name string `json:"name"`
}

type InviteRequest struct {
	UserID       string `json:"user_id"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	LocationName string `json:"location_name,omitempty" example:"Kokand"`
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Create godoc
// @Summary Create a group
// @Tags groups
// @Accept json
// @Produce json
// @Param request body CreateRequest true "Group payload"
// @Success 201 {object} GroupResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Security BearerAuth
// @Router /groups [post]
func (h *Handler) Create(c *fiber.Ctx) error {
	actorID, err := authenticatedUserID(c)
	if err != nil {
		return responses.Error(c, err)
	}
	var request CreateRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.ErrorWithMessage(c, fmt.Errorf("%w: %w", apperror.ErrInvalidData, err), "invalid request")
	}
	group, err := h.service.Create(c.UserContext(), actorID, request.Name)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusCreated, group, "group created")
}

// List godoc
// @Summary List my groups
// @Tags groups
// @Produce json
// @Success 200 {object} GroupsResponse
// @Failure 401 {object} ErrorResponse
// @Security BearerAuth
// @Router /groups [get]
func (h *Handler) List(c *fiber.Ctx) error {
	actorID, err := authenticatedUserID(c)
	if err != nil {
		return responses.Error(c, err)
	}
	groups, err := h.service.List(c.UserContext(), actorID)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, groups, "groups returned")
}

// Get godoc
// @Summary Get group details
// @Tags groups
// @Produce json
// @Param groupID path string true "Group UUID"
// @Success 200 {object} GroupResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /groups/{groupID} [get]
func (h *Handler) Get(c *fiber.Ctx) error {
	actorID, err := authenticatedUserID(c)
	if err != nil {
		return responses.Error(c, err)
	}
	groupID, err := pathUUID(c, "groupID")
	if err != nil {
		return responses.Error(c, err)
	}
	group, err := h.service.Get(c.UserContext(), actorID, groupID)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, group, "group returned")
}

// Invite godoc
// @Summary Create a group invitation
// @Tags group invitations
// @Accept json
// @Produce json
// @Param groupID path string true "Group UUID"
// @Param request body InviteRequest true "Invitation payload; provide user_id or email"
// @Success 201 {object} InvitationResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Security BearerAuth
// @Router /groups/{groupID}/invitations [post]
func (h *Handler) Invite(c *fiber.Ctx) error {
	actorID, err := authenticatedUserID(c)
	if err != nil {
		return responses.Error(c, err)
	}
	groupID, err := pathUUID(c, "groupID")
	if err != nil {
		return responses.Error(c, err)
	}
	var request InviteRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.ErrorWithMessage(c, fmt.Errorf("%w: %w", apperror.ErrInvalidData, err), "invalid request")
	}
	input := InviteInput{Email: request.Email, Role: request.Role, LocationName: request.LocationName}
	if strings.TrimSpace(request.UserID) != "" {
		input.UserID, err = uuid.Parse(strings.TrimSpace(request.UserID))
		if err != nil {
			return responses.Error(c, apperror.ErrInvalidID)
		}
	}
	invitation, err := h.service.Invite(c.UserContext(), actorID, groupID, input)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusCreated, invitation, "invitation created")
}

// ListMembers godoc
// @Summary List active and pending group members
// @Tags groups
// @Produce json
// @Param groupID path string true "Group UUID"
// @Success 200 {object} MembersResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Security BearerAuth
// @Router /groups/{groupID}/members [get]
func (h *Handler) ListMembers(c *fiber.Ctx) error {
	actorID, err := authenticatedUserID(c)
	if err != nil {
		return responses.Error(c, err)
	}
	groupID, err := pathUUID(c, "groupID")
	if err != nil {
		return responses.Error(c, err)
	}
	members, err := h.service.ListMembers(c.UserContext(), actorID, groupID)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, members, "members returned")
}

type InvitationActionRequest struct {
	Action string `json:"action" enums:"accept,reject" example:"accept"`
}

// Action godoc
// @Summary Respond to a group invitation
// @Tags group invitations
// @Accept json
// @Produce json
// @Param invitationID path string true "Invitation UUID"
// @Param request body InvitationActionRequest true "Action payload"
// @Success 200 {object} InvitationResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /invitations/{invitationID}/action [post]
func (h *Handler) Action(c *fiber.Ctx) error {
	actorID, err := authenticatedUserID(c)
	if err != nil {
		return responses.Error(c, err)
	}
	invitationID, err := pathUUID(c, "invitationID")
	if err != nil {
		return responses.Error(c, err)
	}
	var request InvitationActionRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.ErrorWithMessage(c, fmt.Errorf("%w: %w", apperror.ErrInvalidData, err), "invalid request")
	}
	invitation, err := h.service.RespondInvitation(c.UserContext(), actorID, invitationID, strings.ToLower(strings.TrimSpace(request.Action)))
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, invitation, "invitation action processed")
}

func authenticatedUserID(c *fiber.Ctx) (uuid.UUID, error) {
	userID, ok := c.Locals("auth_user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return uuid.Nil, apperror.ErrUnauthorized
	}
	return userID, nil
}

func pathUUID(c *fiber.Ctx, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params(name))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, apperror.ErrInvalidID
	}
	return id, nil
}
