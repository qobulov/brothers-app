package rest

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/apperror"

	"github.com/gofiber/fiber/v2"
	"github.com/qobulov/brothers-app/internal/entities"
	"github.com/qobulov/brothers-app/internal/order/dto"
	"github.com/qobulov/brothers-app/internal/order/usecase"
	responses "github.com/qobulov/brothers-app/pkg/responses"
)

type HttpOrderHandler struct {
	orderUseCase usecase.OrderUseCase
}

func NewHttpOrderHandler(useCase usecase.OrderUseCase) *HttpOrderHandler {
	return &HttpOrderHandler{orderUseCase: useCase}
}

// CreateOrder godoc
// @Summary Create a new order
// @Tags orders
// @Accept json
// @Produce json
// @Param order body orderdto.CreateOrderRequest true "Order payload"
// @Success 201 {object} orderdto.OrderResponse
// @Router /orders [post]
func (h *HttpOrderHandler) CreateOrder(c *fiber.Ctx) error {
	var req orderdto.CreateOrderRequest
	if err := c.BodyParser(&req); err != nil {
		return responses.ErrorWithMessage(c, fmt.Errorf("%w: %w", apperror.ErrInvalidData, err), responses.MessageInvalidRequest)
	}

	order := &entities.Order{Total: req.Total}
	if err := h.orderUseCase.CreateOrder(order); err != nil {
		return responses.Error(c, err)
	}

	return responses.Success(c, fiber.StatusCreated, orderdto.ToOrderResponse(order), responses.MessageRequestProcessed)
}

// FindAllOrders godoc
// @Summary Get all orders
// @Tags orders
// @Produce json
// @Success 200 {array} orderdto.OrderResponse
// @Router /orders [get]
func (h *HttpOrderHandler) FindAllOrders(c *fiber.Ctx) error {
	orders, err := h.orderUseCase.FindAllOrders()
	if err != nil {
		return responses.Error(c, err)
	}

	return responses.Success(c, fiber.StatusOK, orderdto.ToOrderResponseList(orders), responses.MessageRequestProcessed)
}

// FindOrderByID godoc
// @Summary Get order by ID
// @Tags orders
// @Produce json
// @Param id path string true "Order UUID"
// @Success 200 {object} orderdto.OrderResponse
// @Router /orders/{id} [get]
func (h *HttpOrderHandler) FindOrderByID(c *fiber.Ctx) error {
	orderID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return responses.ErrorWithMessage(c, fmt.Errorf("%w: %w", apperror.ErrInvalidID, err), responses.MessageInvalidID)
	}

	order, err := h.orderUseCase.FindOrderByID(orderID)
	if err != nil {
		return responses.Error(c, err)
	}

	return responses.Success(c, fiber.StatusOK, orderdto.ToOrderResponse(order), responses.MessageRequestProcessed)
}

// PatchOrder godoc
// @Summary Update an order partially
// @Tags orders
// @Accept json
// @Produce json
// @Param id path string true "Order UUID"
// @Param order body orderdto.CreateOrderRequest true "Order update payload"
// @Success 200 {object} orderdto.OrderResponse
// @Router /orders/{id} [patch]
func (h *HttpOrderHandler) PatchOrder(c *fiber.Ctx) error {
	orderID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return responses.ErrorWithMessage(c, fmt.Errorf("%w: %w", apperror.ErrInvalidID, err), responses.MessageInvalidID)
	}

	var req orderdto.CreateOrderRequest
	if err := c.BodyParser(&req); err != nil {
		return responses.ErrorWithMessage(c, fmt.Errorf("%w: %w", apperror.ErrInvalidData, err), responses.MessageInvalidRequest)
	}

	order := &entities.Order{Total: req.Total}

	msg, err := validatePatchOrder(order)
	if err != nil {
		return responses.ErrorWithMessage(c, err, msg)
	}

	updatedOrder, err := h.orderUseCase.PatchOrder(orderID, order)
	if err != nil {
		return responses.Error(c, err)
	}

	return responses.Success(c, fiber.StatusOK, orderdto.ToOrderResponse(updatedOrder), responses.MessageRequestProcessed)
}

// DeleteOrder godoc
// @Summary Delete an order by ID
// @Tags orders
// @Produce json
// @Param id path string true "Order UUID"
// @Success 200 {object} responses.MessageResponse
// @Router /orders/{id} [delete]
func (h *HttpOrderHandler) DeleteOrder(c *fiber.Ctx) error {
	orderID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return responses.ErrorWithMessage(c, fmt.Errorf("%w: %w", apperror.ErrInvalidID, err), responses.MessageInvalidID)
	}

	if err := h.orderUseCase.DeleteOrder(orderID); err != nil {
		return responses.Error(c, err)
	}

	return responses.Message(c, fiber.StatusOK, responses.MessageOrderDeleted)
}

func validatePatchOrder(order *entities.Order) (string, error) {

	if order.Total <= 0 {
		return responses.MessageTotalMustBePositive, apperror.ErrInvalidData
	}

	return "", nil
}
