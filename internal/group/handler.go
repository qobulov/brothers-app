package group

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/request"
	"github.com/qobulov/brothers-app/pkg/responses"
)

type Handler struct {
	service *Service
}

type CreateRequest struct {
	Name string `json:"name"`
}

type DeleteRequest struct {
	Confirm bool `json:"confirm"`
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
	actorID, err := request.UserID(c)
	if err != nil {
		return responses.Error(c, err)
	}
	var request CreateRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.InvalidBody(c, err)
	}
	group, err := h.service.Create(c.UserContext(), actorID, request.Name)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusCreated, group, responses.MessageGroupCreated)
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
	actorID, err := request.UserID(c)
	if err != nil {
		return responses.Error(c, err)
	}
	groups, err := h.service.List(c.UserContext(), actorID)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, groups, responses.MessageGroupsReturned)
}

// Delete godoc
// @Summary Soft-delete a group
// @Description Owner-only operation. Requires an explicit confirmation and preserves financial and audit history.
// @Tags groups
// @Accept json
// @Produce json
// @Param groupID path string true "Group UUID"
// @Param request body DeleteRequest true "Deletion confirmation"
// @Success 200 {object} responses.MessageResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /groups/{groupID} [delete]
func (h *Handler) Delete(c *fiber.Ctx) error {
	actorID, err := request.UserID(c)
	if err != nil {
		return responses.Error(c, err)
	}
	groupID, err := request.PathUUID(c, "groupID")
	if err != nil {
		return responses.Error(c, err)
	}
	var request DeleteRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.InvalidBody(c, err)
	}
	if err := h.service.Delete(c.UserContext(), actorID, groupID, request.Confirm); err != nil {
		return responses.Error(c, err)
	}
	return responses.Message(c, fiber.StatusOK, responses.MessageGroupDeleted)
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
	actorID, err := request.UserID(c)
	if err != nil {
		return responses.Error(c, err)
	}
	groupID, err := request.PathUUID(c, "groupID")
	if err != nil {
		return responses.Error(c, err)
	}
	var request InviteRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.InvalidBody(c, err)
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
	return responses.Success(c, fiber.StatusCreated, invitation, responses.MessageInvitationCreated)
}

// ListMembers godoc
// @Summary List active and pending group members
// @Tags groups
// @Produce json
// @Param groupID path string true "Group UUID"
// @Param query query string false "Search by name, username, email, or location"
// @Param role query string false "Role filter" Enums(all,manager,employee,investor)
// @Param status query string false "Status filter" Enums(all,active,pending)
// @Success 200 {object} MembersResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Security BearerAuth
// @Router /groups/{groupID}/members [get]
func (h *Handler) ListMembers(c *fiber.Ctx) error {
	actorID, err := request.UserID(c)
	if err != nil {
		return responses.Error(c, err)
	}
	groupID, err := request.PathUUID(c, "groupID")
	if err != nil {
		return responses.Error(c, err)
	}
	members, err := h.service.ListMembers(c.UserContext(), actorID, groupID, ListMembersInput{
		Query:  c.Query("query"),
		Role:   c.Query("role"),
		Status: c.Query("status"),
	})
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, members, responses.MessageMembersReturned)
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
	actorID, err := request.UserID(c)
	if err != nil {
		return responses.Error(c, err)
	}
	invitationID, err := request.PathUUID(c, "invitationID")
	if err != nil {
		return responses.Error(c, err)
	}
	var request InvitationActionRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.InvalidBody(c, err)
	}
	invitation, err := h.service.RespondInvitation(c.UserContext(), actorID, invitationID, strings.ToLower(strings.TrimSpace(request.Action)))
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, invitation, responses.MessageInvitationAction)
}
