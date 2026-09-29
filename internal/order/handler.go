package order

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/responses"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type CreateRequest struct {
	GiverUserID           string `json:"giver_user_id" format:"uuid"`
	GiverCustomerPhone    string `json:"giver_customer_phone" example:"+998901234567"`
	ReceiverUserID        string `json:"receiver_user_id" format:"uuid"`
	ReceiverCustomerPhone string `json:"receiver_customer_phone" example:"+998907654321"`
	AmountUSD             int64  `json:"amount_usd" example:"7000"`
	FeeUZS                int64  `json:"fee_uzs" example:"50000"`
}

// EditRequest changes only the fields that are present.
type EditRequest struct {
	GiverUserID           *string `json:"giver_user_id,omitempty" format:"uuid"`
	GiverCustomerPhone    *string `json:"giver_customer_phone,omitempty" example:"+998901234567"`
	ReceiverUserID        *string `json:"receiver_user_id,omitempty" format:"uuid"`
	ReceiverCustomerPhone *string `json:"receiver_customer_phone,omitempty" example:"+998907654321"`
	AmountUSD             *int64  `json:"amount_usd,omitempty" example:"6800"`
	FeeUZS                *int64  `json:"fee_uzs,omitempty" example:"40000"`
}

type ConfirmRequest struct {
	AmountUSD int64 `json:"amount_usd" example:"7000"`
	FeeUZS    int64 `json:"fee_uzs" example:"50000"`
}

// Create godoc
// @Summary Create an order between two employees
// @Description Managers create orders for any two employees; an employee only for an order they take part in.
// @Tags orders
// @Accept json
// @Produce json
// @Param groupID path string true "Group UUID"
// @Param request body CreateRequest true "Order payload"
// @Success 201 {object} OrderResponse
// @Failure 400 {object} responses.ErrorResponse
// @Failure 401 {object} responses.ErrorResponse
// @Failure 403 {object} responses.ErrorResponse
// @Failure 404 {object} responses.ErrorResponse
// @Security BearerAuth
// @Router /groups/{groupID}/orders [post]
func (h *Handler) Create(c *fiber.Ctx) error {
	actorID, groupID, err := actorAndGroup(c)
	if err != nil {
		return responses.Error(c, err)
	}
	var request CreateRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.ErrorWithMessage(c, fmt.Errorf("%w: %w", apperror.ErrInvalidData, err), responses.MessageInvalidRequest)
	}
	input, err := request.input()
	if err != nil {
		return responses.Error(c, err)
	}
	created, err := h.service.Create(c.UserContext(), actorID, groupID, input)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusCreated, created, responses.MessageOrderCreated)
}

// List godoc
// @Summary List group orders
// @Description Newest first. Employees only see orders they take part in.
// @Tags orders
// @Produce json
// @Param groupID path string true "Group UUID"
// @Param status query string false "Status filter" Enums(pending,completed,cancelled)
// @Param limit query int false "Page size" default(50) minimum(1) maximum(100)
// @Param offset query int false "Number of orders to skip" default(0) minimum(0) maximum(10000)
// @Success 200 {object} OrdersResponse
// @Failure 400 {object} responses.ErrorResponse
// @Failure 401 {object} responses.ErrorResponse
// @Failure 404 {object} responses.ErrorResponse
// @Security BearerAuth
// @Router /groups/{groupID}/orders [get]
func (h *Handler) List(c *fiber.Ctx) error {
	actorID, groupID, err := actorAndGroup(c)
	if err != nil {
		return responses.Error(c, err)
	}
	limit, err := optionalInt(c, "limit")
	if err != nil {
		return responses.Error(c, err)
	}
	offset, err := optionalInt(c, "offset")
	if err != nil {
		return responses.Error(c, err)
	}
	items, err := h.service.List(c.UserContext(), actorID, groupID, ListInput{Status: c.Query("status"), Limit: limit, Offset: offset})
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, items, responses.MessageOrdersReturned)
}

// Get godoc
// @Summary Get an order
// @Description The other party's confirmed amount is hidden until you confirm; their fee is never shown to a party.
// @Tags orders
// @Produce json
// @Param groupID path string true "Group UUID"
// @Param orderID path string true "Order UUID"
// @Success 200 {object} OrderResponse
// @Failure 400 {object} responses.ErrorResponse
// @Failure 401 {object} responses.ErrorResponse
// @Failure 404 {object} responses.ErrorResponse
// @Security BearerAuth
// @Router /groups/{groupID}/orders/{orderID} [get]
func (h *Handler) Get(c *fiber.Ctx) error {
	actorID, groupID, orderID, err := actorGroupAndOrder(c)
	if err != nil {
		return responses.Error(c, err)
	}
	found, err := h.service.Get(c.UserContext(), actorID, groupID, orderID)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, found, responses.MessageRequestProcessed)
}

// Edit godoc
// @Summary Edit a pending order
// @Description Any change clears both confirmations. Completed orders cannot be edited.
// @Tags orders
// @Accept json
// @Produce json
// @Param groupID path string true "Group UUID"
// @Param orderID path string true "Order UUID"
// @Param request body EditRequest true "Fields to change"
// @Success 200 {object} OrderResponse
// @Failure 400 {object} responses.ErrorResponse
// @Failure 401 {object} responses.ErrorResponse
// @Failure 403 {object} responses.ErrorResponse
// @Failure 404 {object} responses.ErrorResponse
// @Failure 409 {object} responses.ErrorResponse
// @Security BearerAuth
// @Router /groups/{groupID}/orders/{orderID} [patch]
func (h *Handler) Edit(c *fiber.Ctx) error {
	actorID, groupID, orderID, err := actorGroupAndOrder(c)
	if err != nil {
		return responses.Error(c, err)
	}
	var request EditRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.ErrorWithMessage(c, fmt.Errorf("%w: %w", apperror.ErrInvalidData, err), responses.MessageInvalidRequest)
	}
	input, err := request.input()
	if err != nil {
		return responses.Error(c, err)
	}
	edited, err := h.service.Edit(c.UserContext(), actorID, groupID, orderID, input)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, edited, responses.MessageOrderUpdated)
}

// Confirm godoc
// @Summary Confirm or correct your amount for an order
// @Description Only the giver or receiver. When both amounts match the order completes and balances and profits update.
// @Tags orders
// @Accept json
// @Produce json
// @Param groupID path string true "Group UUID"
// @Param orderID path string true "Order UUID"
// @Param request body ConfirmRequest true "Amount you received or paid, and the fee you collected"
// @Success 200 {object} OrderResponse
// @Failure 400 {object} responses.ErrorResponse
// @Failure 401 {object} responses.ErrorResponse
// @Failure 403 {object} responses.ErrorResponse
// @Failure 404 {object} responses.ErrorResponse
// @Failure 409 {object} responses.ErrorResponse
// @Security BearerAuth
// @Router /groups/{groupID}/orders/{orderID}/confirmations [post]
func (h *Handler) Confirm(c *fiber.Ctx) error {
	actorID, groupID, orderID, err := actorGroupAndOrder(c)
	if err != nil {
		return responses.Error(c, err)
	}
	var request ConfirmRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.ErrorWithMessage(c, fmt.Errorf("%w: %w", apperror.ErrInvalidData, err), responses.MessageInvalidRequest)
	}
	confirmed, err := h.service.Confirm(c.UserContext(), actorID, groupID, orderID, ConfirmInput{AmountUSD: request.AmountUSD, FeeUZS: request.FeeUZS})
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, confirmed, responses.MessageOrderConfirmed)
}

// Events godoc
// @Summary Order history
// @Tags orders
// @Produce json
// @Param groupID path string true "Group UUID"
// @Param orderID path string true "Order UUID"
// @Success 200 {object} OrderEventsResponse
// @Failure 400 {object} responses.ErrorResponse
// @Failure 401 {object} responses.ErrorResponse
// @Failure 404 {object} responses.ErrorResponse
// @Security BearerAuth
// @Router /groups/{groupID}/orders/{orderID}/events [get]
func (h *Handler) Events(c *fiber.Ctx) error {
	actorID, groupID, orderID, err := actorGroupAndOrder(c)
	if err != nil {
		return responses.Error(c, err)
	}
	events, err := h.service.ListEvents(c.UserContext(), actorID, groupID, orderID)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, events, responses.MessageRequestProcessed)
}

func (r CreateRequest) input() (CreateInput, error) {
	giverID, err := parseUserID(r.GiverUserID)
	if err != nil {
		return CreateInput{}, err
	}
	receiverID, err := parseUserID(r.ReceiverUserID)
	if err != nil {
		return CreateInput{}, err
	}
	return CreateInput{
		GiverUserID: giverID, GiverCustomerPhone: r.GiverCustomerPhone,
		ReceiverUserID: receiverID, ReceiverCustomerPhone: r.ReceiverCustomerPhone,
		AmountUSD: r.AmountUSD, FeeUZS: r.FeeUZS,
	}, nil
}

func (r EditRequest) input() (EditInput, error) {
	giverID, err := parseOptionalUserID(r.GiverUserID)
	if err != nil {
		return EditInput{}, err
	}
	receiverID, err := parseOptionalUserID(r.ReceiverUserID)
	if err != nil {
		return EditInput{}, err
	}
	return EditInput{
		GiverUserID: giverID, GiverCustomerPhone: r.GiverCustomerPhone,
		ReceiverUserID: receiverID, ReceiverCustomerPhone: r.ReceiverCustomerPhone,
		AmountUSD: r.AmountUSD, FeeUZS: r.FeeUZS,
	}, nil
}

func parseUserID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, apperror.ErrInvalidID
	}
	return id, nil
}

func parseOptionalUserID(value *string) (*uuid.UUID, error) {
	if value == nil {
		return nil, nil
	}
	id, err := parseUserID(*value)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func actorAndGroup(c *fiber.Ctx) (uuid.UUID, uuid.UUID, error) {
	actorID, ok := c.Locals("auth_user_id").(uuid.UUID)
	if !ok || actorID == uuid.Nil {
		return uuid.Nil, uuid.Nil, apperror.ErrUnauthorized
	}
	groupID, err := pathUUID(c, "groupID")
	return actorID, groupID, err
}

func actorGroupAndOrder(c *fiber.Ctx) (uuid.UUID, uuid.UUID, uuid.UUID, error) {
	actorID, groupID, err := actorAndGroup(c)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, err
	}
	orderID, err := pathUUID(c, "orderID")
	return actorID, groupID, orderID, err
}

func pathUUID(c *fiber.Ctx, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params(name))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, apperror.ErrInvalidID
	}
	return id, nil
}

// optionalInt reads an integer query parameter; a missing one is 0 and the
// service applies its default.
func optionalInt(c *fiber.Ctx, name string) (int, error) {
	raw := c.Query(name)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%w: %s must be an integer", apperror.ErrInvalidData, name)
	}
	return value, nil
}

type CancellationRequest struct {
	Reason string `json:"reason,omitempty" example:"Customer changed their mind"`
}

type CancellationActionRequest struct {
	Action string `json:"action" enums:"approve,reject" example:"approve"`
}

// RequestCancellation godoc
// @Summary Request order cancellation
// @Description Only the giver or receiver, for a pending or completed order. The requester counts as approved; the other party must approve.
// @Tags orders
// @Accept json
// @Produce json
// @Param groupID path string true "Group UUID"
// @Param orderID path string true "Order UUID"
// @Param request body CancellationRequest false "Optional reason"
// @Success 201 {object} OrderResponse
// @Failure 400 {object} responses.ErrorResponse
// @Failure 401 {object} responses.ErrorResponse
// @Failure 403 {object} responses.ErrorResponse
// @Failure 404 {object} responses.ErrorResponse
// @Failure 409 {object} responses.ErrorResponse
// @Security BearerAuth
// @Router /groups/{groupID}/orders/{orderID}/cancellation [post]
func (h *Handler) RequestCancellation(c *fiber.Ctx) error {
	actorID, groupID, orderID, err := actorGroupAndOrder(c)
	if err != nil {
		return responses.Error(c, err)
	}
	var request CancellationRequest
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&request); err != nil {
			return responses.ErrorWithMessage(c, fmt.Errorf("%w: %w", apperror.ErrInvalidData, err), responses.MessageInvalidRequest)
		}
	}
	requested, err := h.service.RequestCancellation(c.UserContext(), actorID, groupID, orderID, request.Reason)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusCreated, requested, responses.MessageCancellationRequested)
}

// RespondCancellation godoc
// @Summary Approve or reject a cancellation request
// @Description approve: only the other party; cancels the order and reverses a completed order's balances and profits. reject: either party ("Keep Order", or the requester withdrawing).
// @Tags orders
// @Accept json
// @Produce json
// @Param groupID path string true "Group UUID"
// @Param orderID path string true "Order UUID"
// @Param request body CancellationActionRequest true "Action"
// @Success 200 {object} OrderResponse
// @Failure 400 {object} responses.ErrorResponse
// @Failure 401 {object} responses.ErrorResponse
// @Failure 403 {object} responses.ErrorResponse
// @Failure 404 {object} responses.ErrorResponse
// @Failure 409 {object} responses.ErrorResponse
// @Security BearerAuth
// @Router /groups/{groupID}/orders/{orderID}/cancellation/action [post]
func (h *Handler) RespondCancellation(c *fiber.Ctx) error {
	actorID, groupID, orderID, err := actorGroupAndOrder(c)
	if err != nil {
		return responses.Error(c, err)
	}
	var request CancellationActionRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.ErrorWithMessage(c, fmt.Errorf("%w: %w", apperror.ErrInvalidData, err), responses.MessageInvalidRequest)
	}
	responded, err := h.service.RespondCancellation(c.UserContext(), actorID, groupID, orderID, request.Action)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, responded, responses.MessageCancellationProcessed)
}
