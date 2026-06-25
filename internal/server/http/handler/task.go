package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/server/http/middleware"
	"github.com/ramisoul84/task-management/internal/service"
	"github.com/ramisoul84/task-management/pkg/validator"
)

type TaskHandler struct {
	svc       service.TaskService
	validator *validator.Validator
}

func NewTaskHandler(svc service.TaskService, validator *validator.Validator) *TaskHandler {
	return &TaskHandler{
		svc:       svc,
		validator: validator,
	}
}

// POST /api/v1/tasks
func (h *TaskHandler) Create(c *fiber.Ctx) error {
	userID, ok := middleware.UserID(c)
	if !ok {
		return fiber.ErrUnauthorized
	}

	var req domain.CreateTaskRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	if err := h.validator.Validate(req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	task, err := h.svc.CreateTask(c.UserContext(), userID, &req)
	if err != nil {
		return MapError(err)
	}

	return c.Status(fiber.StatusCreated).JSON(task)
}

func (h *TaskHandler) ListByTeam(c *fiber.Ctx) error {
	userID, ok := middleware.UserID(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}

	var query domain.TaskListQuery
	if err := c.QueryParser(&query); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid query parameters")
	}

	// Set default values
	if query.Limit == 0 {
		query.Limit = 20
	}
	if query.Limit > 100 {
		query.Limit = 100
	}

	// Validate query
	if err := h.validator.Validate(query); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	// Call service
	result, err := h.svc.ListTasksByTeam(c.UserContext(), userID, &query)
	if err != nil {
		return MapError(err)
	}

	return c.JSON(result)
}
