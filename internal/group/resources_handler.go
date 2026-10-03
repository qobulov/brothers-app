package group

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/request"
	"github.com/qobulov/brothers-app/pkg/responses"
)

type CreateLocationRequest struct {
	Name       string `json:"name" example:"Tashkent"`
	EmployeeID string `json:"employee_id,omitempty" format:"uuid"`
}

// ListLocations godoc
// @Summary List group locations
// @Tags group locations
// @Produce json
// @Param groupID path string true "Group UUID"
// @Success 200 {object} LocationsResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /groups/{groupID}/locations [get]
func (h *Handler) ListLocations(c *fiber.Ctx) error {
	actorID, err := request.UserID(c)
	if err != nil {
		return responses.Error(c, err)
	}
	groupID, err := request.PathUUID(c, "groupID")
	if err != nil {
		return responses.Error(c, err)
	}
	locations, err := h.service.ListLocations(c.UserContext(), actorID, groupID)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, locations, responses.MessageRequestProcessed)
}

// CreateLocation godoc
// @Summary Create a group location
// @Tags group locations
// @Accept json
// @Produce json
// @Param groupID path string true "Group UUID"
// @Param request body CreateLocationRequest true "Location payload"
// @Success 201 {object} LocationResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Security BearerAuth
// @Router /groups/{groupID}/locations [post]
func (h *Handler) CreateLocation(c *fiber.Ctx) error {
	actorID, err := request.UserID(c)
	if err != nil {
		return responses.Error(c, err)
	}
	groupID, err := request.PathUUID(c, "groupID")
	if err != nil {
		return responses.Error(c, err)
	}
	var request CreateLocationRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.InvalidBody(c, err)
	}
	employeeID, err := optionalUUID(request.EmployeeID)
	if err != nil {
		return responses.Error(c, err)
	}
	location, err := h.service.CreateLocation(c.UserContext(), actorID, groupID, CreateLocationInput{
		Name:       request.Name,
		EmployeeID: employeeID,
	})
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusCreated, location, responses.MessageRequestProcessed)
}

// DeleteLocation godoc
// @Summary Soft-delete an unassigned group location
// @Tags group locations
// @Produce json
// @Param groupID path string true "Group UUID"
// @Param locationID path string true "Location UUID"
// @Success 200 {object} responses.MessageResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Security BearerAuth
// @Router /groups/{groupID}/locations/{locationID} [delete]
func (h *Handler) DeleteLocation(c *fiber.Ctx) error {
	actorID, err := request.UserID(c)
	if err != nil {
		return responses.Error(c, err)
	}
	groupID, err := request.PathUUID(c, "groupID")
	if err != nil {
		return responses.Error(c, err)
	}
	locationID, err := request.PathUUID(c, "locationID")
	if err != nil {
		return responses.Error(c, err)
	}
	if err := h.service.DeleteLocation(c.UserContext(), actorID, groupID, locationID); err != nil {
		return responses.Error(c, err)
	}
	return responses.Message(c, fiber.StatusOK, responses.MessageRequestProcessed)
}

// ListCustomers godoc
// @Summary List group customers
// @Description This resource is read-only. The optional query parameter filters by phone.
// @Tags group customers
// @Produce json
// @Param groupID path string true "Group UUID"
// @Param query query string false "Phone search"
// @Success 200 {object} CustomersResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /groups/{groupID}/customers [get]
func (h *Handler) ListCustomers(c *fiber.Ctx) error {
	actorID, err := request.UserID(c)
	if err != nil {
		return responses.Error(c, err)
	}
	groupID, err := request.PathUUID(c, "groupID")
	if err != nil {
		return responses.Error(c, err)
	}
	customers, err := h.service.ListCustomers(c.UserContext(), actorID, groupID, c.Query("query"))
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, customers, responses.MessageRequestProcessed)
}

func optionalUUID(value string) (uuid.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return uuid.Nil, nil
	}
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, apperror.ErrInvalidID
	}
	return id, nil
}
