package debt

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/request"
	"github.com/qobulov/brothers-app/pkg/responses"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type CreateRequest struct {
	Direction   string `json:"direction" enums:"they_owe_me,i_owe" example:"they_owe_me"`
	PersonName  string `json:"person_name" example:"Akmal"`
	PersonPhone string `json:"person_phone,omitempty" example:"+998907774422"`
	Currency    string `json:"currency" enums:"USD,UZS" example:"USD"`
	Amount      int64  `json:"amount" example:"1500"`
}

type RepaymentRequest struct {
	Amount int64 `json:"amount" example:"600"`
}

// Summary godoc
// @Summary Debt totals
// @Description Remaining amounts of active debts, per currency and direction. Currencies are never converted.
// @Tags debts
// @Produce json
// @Success 200 {object} SummaryResponse
// @Failure 401 {object} responses.ErrorResponse
// @Security BearerAuth
// @Router /debts/summary [get]
func (h *Handler) Summary(c *fiber.Ctx) error {
	ownerID, err := request.UserID(c)
	if err != nil {
		return responses.Error(c, err)
	}
	summary, err := h.service.Summary(c.UserContext(), ownerID)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, summary, responses.MessageRequestProcessed)
}

// List godoc
// @Summary List my debts
// @Description Newest first. Only the caller's own debts.
// @Tags debts
// @Produce json
// @Param direction query string false "Direction" Enums(they_owe_me,i_owe)
// @Param status query string false "Status" Enums(active,completed,all) default(active)
// @Param query query string false "Search by name or phone" maxlength(100)
// @Param limit query int false "Page size" default(50) minimum(1) maximum(100)
// @Param offset query int false "Number of debts to skip" default(0) minimum(0) maximum(10000)
// @Success 200 {object} DebtsResponse
// @Failure 400 {object} responses.ErrorResponse
// @Failure 401 {object} responses.ErrorResponse
// @Security BearerAuth
// @Router /debts [get]
func (h *Handler) List(c *fiber.Ctx) error {
	ownerID, err := request.UserID(c)
	if err != nil {
		return responses.Error(c, err)
	}
	page, err := request.Page(c)
	if err != nil {
		return responses.Error(c, err)
	}
	debts, err := h.service.List(c.UserContext(), ownerID, ListInput{
		Direction: c.Query("direction"), Status: c.Query("status"), Query: c.Query("query"),
		Limit: page.Limit, Offset: page.Offset,
	})
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, debts, responses.MessageRequestProcessed)
}

// Create godoc
// @Summary Add a debt
// @Description A private debt visible only to the caller. It does not affect any group balance.
// @Tags debts
// @Accept json
// @Produce json
// @Param request body CreateRequest true "Debt"
// @Success 201 {object} DebtResponse
// @Failure 400 {object} responses.ErrorResponse
// @Failure 401 {object} responses.ErrorResponse
// @Security BearerAuth
// @Router /debts [post]
func (h *Handler) Create(c *fiber.Ctx) error {
	ownerID, err := request.UserID(c)
	if err != nil {
		return responses.Error(c, err)
	}
	var request CreateRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.InvalidBody(c, err)
	}
	created, err := h.service.Create(c.UserContext(), ownerID, CreateInput(request))
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusCreated, created, responses.MessageDebtCreated)
}

// Get godoc
// @Summary Get a debt
// @Tags debts
// @Produce json
// @Param debtID path string true "Debt UUID"
// @Success 200 {object} DebtResponse
// @Failure 400 {object} responses.ErrorResponse
// @Failure 401 {object} responses.ErrorResponse
// @Failure 404 {object} responses.ErrorResponse
// @Security BearerAuth
// @Router /debts/{debtID} [get]
func (h *Handler) Get(c *fiber.Ctx) error {
	ownerID, debtID, err := ownerAndDebt(c)
	if err != nil {
		return responses.Error(c, err)
	}
	found, err := h.service.Get(c.UserContext(), ownerID, debtID)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, found, responses.MessageRequestProcessed)
}

// Repay godoc
// @Summary Record a repayment
// @Description Partial repayments are supported. Reaching zero completes the debt.
// @Tags debts
// @Accept json
// @Produce json
// @Param debtID path string true "Debt UUID"
// @Param request body RepaymentRequest true "Repayment"
// @Success 201 {object} DebtResponse
// @Failure 400 {object} responses.ErrorResponse
// @Failure 401 {object} responses.ErrorResponse
// @Failure 404 {object} responses.ErrorResponse
// @Failure 409 {object} responses.ErrorResponse
// @Security BearerAuth
// @Router /debts/{debtID}/repayments [post]
func (h *Handler) Repay(c *fiber.Ctx) error {
	ownerID, debtID, err := ownerAndDebt(c)
	if err != nil {
		return responses.Error(c, err)
	}
	var request RepaymentRequest
	if err := c.BodyParser(&request); err != nil {
		return responses.InvalidBody(c, err)
	}
	updated, err := h.service.Repay(c.UserContext(), ownerID, debtID, request.Amount)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusCreated, updated, responses.MessageRepaymentRecorded)
}

// Repayments godoc
// @Summary Repayment history of a debt
// @Tags debts
// @Produce json
// @Param debtID path string true "Debt UUID"
// @Param limit query int false "Page size" default(50) minimum(1) maximum(100)
// @Param offset query int false "Number of repayments to skip" default(0) minimum(0) maximum(10000)
// @Success 200 {object} RepaymentsResponse
// @Failure 400 {object} responses.ErrorResponse
// @Failure 401 {object} responses.ErrorResponse
// @Failure 404 {object} responses.ErrorResponse
// @Security BearerAuth
// @Router /debts/{debtID}/repayments [get]
func (h *Handler) Repayments(c *fiber.Ctx) error {
	ownerID, debtID, err := ownerAndDebt(c)
	if err != nil {
		return responses.Error(c, err)
	}
	page, err := request.Page(c)
	if err != nil {
		return responses.Error(c, err)
	}
	repayments, err := h.service.ListRepayments(c.UserContext(), ownerID, debtID, page)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, repayments, responses.MessageRequestProcessed)
}

// Complete godoc
// @Summary Mark a debt as completed
// @Description Closes an active debt; the remaining amount is kept as is.
// @Tags debts
// @Produce json
// @Param debtID path string true "Debt UUID"
// @Success 200 {object} DebtResponse
// @Failure 400 {object} responses.ErrorResponse
// @Failure 401 {object} responses.ErrorResponse
// @Failure 404 {object} responses.ErrorResponse
// @Failure 409 {object} responses.ErrorResponse
// @Security BearerAuth
// @Router /debts/{debtID}/complete [post]
func (h *Handler) Complete(c *fiber.Ctx) error {
	ownerID, debtID, err := ownerAndDebt(c)
	if err != nil {
		return responses.Error(c, err)
	}
	completed, err := h.service.Complete(c.UserContext(), ownerID, debtID)
	if err != nil {
		return responses.Error(c, err)
	}
	return responses.Success(c, fiber.StatusOK, completed, responses.MessageDebtCompleted)
}

// Delete godoc
// @Summary Delete a debt
// @Tags debts
// @Produce json
// @Param debtID path string true "Debt UUID"
// @Success 200 {object} responses.MessageResponse
// @Failure 400 {object} responses.ErrorResponse
// @Failure 401 {object} responses.ErrorResponse
// @Failure 404 {object} responses.ErrorResponse
// @Security BearerAuth
// @Router /debts/{debtID} [delete]
func (h *Handler) Delete(c *fiber.Ctx) error {
	ownerID, debtID, err := ownerAndDebt(c)
	if err != nil {
		return responses.Error(c, err)
	}
	if err := h.service.Delete(c.UserContext(), ownerID, debtID); err != nil {
		return responses.Error(c, err)
	}
	return responses.Message(c, fiber.StatusOK, responses.MessageDebtDeleted)
}

func ownerAndDebt(c *fiber.Ctx) (uuid.UUID, uuid.UUID, error) {
	ownerID, err := request.UserID(c)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	debtID, err := request.PathUUID(c, "debtID")
	return ownerID, debtID, err
}
