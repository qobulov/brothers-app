package rest

import (
	"strconv"

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
// @Param order body entities.Order true "Order payload"
// @Success 201 {object} entities.Order
// @Router /orders [post]
func (h *HttpOrderHandler) CreateOrder(c *fiber.Ctx) error {
	var req dto.CreateOrderRequest
	if err := c.BodyParser(&req); err != nil {
		return responses.ErrorWithMessage(c, err, "invalid request")
	}

	order := &entities.Order{Total: req.Total}
	if err := h.orderUseCase.CreateOrder(order); err != nil {
		return responses.Error(c, err)
	}

	return responses.Success(c, fiber.StatusCreated, dto.ToOrderResponse(order), "Запрос успешно обработан")
}

// FindAllOrders godoc
// @Summary Get all orders
// @Tags orders
// @Produce json
// @Success 200 {array} entities.Order
// @Router /orders [get]
func (h *HttpOrderHandler) FindAllOrders(c *fiber.Ctx) error {
	orders, err := h.orderUseCase.FindAllOrders()
	if err != nil {
		return responses.Error(c, err)
	}

	return responses.Success(c, fiber.StatusOK, dto.ToOrderResponseList(orders), "Запрос успешно обработан")
}

// FindOrderByID godoc
// @Summary Get order by ID
// @Tags orders
// @Produce json
// @Param id path int true "Order ID"
// @Success 200 {object} entities.Order
// @Router /orders/{id} [get]
func (h *HttpOrderHandler) FindOrderByID(c *fiber.Ctx) error {
	id := c.Params("id")
	orderID, err := strconv.Atoi(id)
	if err != nil {
		return responses.ErrorWithMessage(c, err, "invalid id")
	}

	order, err := h.orderUseCase.FindOrderByID(orderID)
	if err != nil {
		return responses.Error(c, err)
	}

	return responses.Success(c, fiber.StatusOK, dto.ToOrderResponse(order), "Запрос успешно обработан")
}

// PatchOrder godoc
// @Summary Update an order partially
// @Tags orders
// @Accept json
// @Produce json
// @Param id path int true "Order ID"
// @Param order body entities.Order true "Order update payload"
// @Success 200 {object} entities.Order
// @Router /orders/{id} [patch]
func (h *HttpOrderHandler) PatchOrder(c *fiber.Ctx) error {
	id := c.Params("id")
	orderID, err := strconv.Atoi(id)
	if err != nil {
		return responses.ErrorWithMessage(c, err, "invalid id")
	}

	var req dto.CreateOrderRequest
	if err := c.BodyParser(&req); err != nil {
		return responses.ErrorWithMessage(c, err, "invalid request")
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

	return responses.Success(c, fiber.StatusOK, dto.ToOrderResponse(updatedOrder), "Запрос успешно обработан")
}

// DeleteOrder godoc
// @Summary Delete an order by ID
// @Tags orders
// @Produce json
// @Param id path int true "Order ID"
// @Success 200 {object} responses.MessageResponse
// @Router /orders/{id} [delete]
func (h *HttpOrderHandler) DeleteOrder(c *fiber.Ctx) error {
	id := c.Params("id")
	orderID, err := strconv.Atoi(id)
	if err != nil {
		return responses.ErrorWithMessage(c, err, "invalid id")
	}

	if err := h.orderUseCase.DeleteOrder(orderID); err != nil {
		return responses.Error(c, err)
	}

	return responses.Message(c, fiber.StatusOK, "order deleted")
}

func validatePatchOrder(order *entities.Order) (string, error) {

	if order.Total <= 0 {
		return "total must be positive", apperror.ErrInvalidData
	}

	return "", nil
}
