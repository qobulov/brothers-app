package group

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/request"
	"github.com/qobulov/brothers-app/pkg/responses"
)

// EditMemberRequest changes only the fields that are present. An empty
// location_id unassigns the member's location.
type EditMemberRequest struct {
	Role       *string `json:"role,omitempty" enums:"employee,manager,investor" example:"manager"`
	LocationID *string `json:"location_id,omitempty" format:"uuid"`
}

type AdjustBalanceRequest struct {
	NewBalanceUSD int64  `json:"new_balance_usd" example:"7500"`
	Reason        string `json:"reason,omitempty" example:"Cash correction"`
}

// GetMember godoc
// @Summary Get one group member
// @Description Owner, managers and investors can open any member; an employee only themselves. balance_usd and profit_uzs are returned for employees only.
// @Tags group members
// @Produce json
// @Param groupID path string true "Group UUID"
// @Param userID path string true "Member's user UUID"
// @Success 200 {object} MemberDetailResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /groups/{groupID}/members/{userID} [get]
func (h *Handler) GetMember(c *fiber.Ctx) error {
	ids, err := memberRequestIDs(c)
	if err != nil {
		return responses.Error(c, err)
	}
	member, err := h.service.GetMember(c.UserContext(), ids.actor, ids.group, ids.user)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, member, responses.MessageRequestProcessed)
}

// EditMember godoc
// @Summary Change a member's role or location
// @Description Role: owner only; leaving the employee role needs a $0 balance and no active orders. Location: any manager, employees only; an empty location_id unassigns it.
// @Tags group members
// @Accept json
// @Produce json
// @Param groupID path string true "Group UUID"
// @Param userID path string true "Member's user UUID"
// @Param request body EditMemberRequest true "Fields to change"
// @Success 200 {object} MemberDetailResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Security BearerAuth
// @Router /groups/{groupID}/members/{userID} [patch]
func (h *Handler) EditMember(c *fiber.Ctx) error {
	ids, err := memberRequestIDs(c)
	if err != nil {
		return responses.Error(c, err)
	}
	var request EditMemberRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.InvalidBody(c, err)
	}
	input := EditMemberInput{Role: request.Role}
	if request.LocationID != nil {
		locationID, err := optionalUUID(*request.LocationID)
		if err != nil {
			return responses.Error(c, err)
		}
		input.LocationID = &locationID
	}
	member, err := h.service.EditMember(c.UserContext(), ids.actor, ids.group, ids.user, input)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, member, responses.MessageMemberUpdated)
}

// RemoveMember godoc
// @Summary Remove a member from the group
// @Description Any manager removes an employee; only the owner removes a manager or investor. Requires a $0 balance and no active orders. The member's old orders can no longer be cancelled.
// @Tags group members
// @Produce json
// @Param groupID path string true "Group UUID"
// @Param userID path string true "Member's user UUID"
// @Success 200 {object} responses.MessageResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Security BearerAuth
// @Router /groups/{groupID}/members/{userID} [delete]
func (h *Handler) RemoveMember(c *fiber.Ctx) error {
	ids, err := memberRequestIDs(c)
	if err != nil {
		return responses.Error(c, err)
	}
	if err := h.service.RemoveMember(c.UserContext(), ids.actor, ids.group, ids.user); err != nil {
		return responses.Error(c, err)
	}
	return responses.Message(c, fiber.StatusOK, responses.MessageMemberRemoved)
}

// AdjustBalance godoc
// @Summary Set an employee's balance
// @Description Any manager. Records the old and new balance permanently; a wrong adjustment is corrected with another one.
// @Tags group members
// @Accept json
// @Produce json
// @Param groupID path string true "Group UUID"
// @Param userID path string true "Employee's user UUID"
// @Param request body AdjustBalanceRequest true "New balance and optional reason"
// @Success 201 {object} BalanceAdjustmentResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /groups/{groupID}/members/{userID}/balance-adjustments [post]
func (h *Handler) AdjustBalance(c *fiber.Ctx) error {
	ids, err := memberRequestIDs(c)
	if err != nil {
		return responses.Error(c, err)
	}
	var request AdjustBalanceRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.InvalidBody(c, err)
	}
	adjustment, err := h.service.AdjustBalance(c.UserContext(), ids.actor, ids.group, ids.user, AdjustBalanceInput(request))
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusCreated, adjustment, responses.MessageBalanceAdjusted)
}

// BalanceHistory godoc
// @Summary Balance history of a member
// @Description Manual adjustments only, newest first, with the member and their current balance.
// @Tags group members
// @Produce json
// @Param groupID path string true "Group UUID"
// @Param userID path string true "Member's user UUID"
// @Param limit query int false "Page size" default(20) minimum(1) maximum(100)
// @Param offset query int false "Number of adjustments to skip" default(0) minimum(0) maximum(10000)
// @Success 200 {object} BalanceHistoryResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /groups/{groupID}/members/{userID}/balance-adjustments [get]
func (h *Handler) BalanceHistory(c *fiber.Ctx) error {
	ids, err := memberRequestIDs(c)
	if err != nil {
		return responses.Error(c, err)
	}
	page, err := request.Page(c)
	if err != nil {
		return responses.Error(c, err)
	}
	history, err := h.service.ListBalanceAdjustments(c.UserContext(), ids.actor, ids.group, ids.user, page)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, history, responses.MessageRequestProcessed)
}

type memberIDs struct {
	actor, group, user uuid.UUID
}

func memberRequestIDs(c *fiber.Ctx) (memberIDs, error) {
	actorID, err := request.UserID(c)
	if err != nil {
		return memberIDs{}, err
	}
	groupID, err := request.PathUUID(c, "groupID")
	if err != nil {
		return memberIDs{}, err
	}
	userID, err := request.PathUUID(c, "userID")
	if err != nil {
		return memberIDs{}, err
	}
	return memberIDs{actor: actorID, group: groupID, user: userID}, nil
}
