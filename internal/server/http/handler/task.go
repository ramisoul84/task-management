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

// GET /api/v1/tasks?team_id=1&status=todo&assignee_id=5&limit=20&offset=0
func (h *TaskHandler) ListByTeam(c *fiber.Ctx) error {
	userID, ok := middleware.UserID(c)
	if !ok {
		return fiber.ErrUnauthorized
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

	if err := h.validator.Validate(query); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	result, err := h.svc.ListTasksByTeam(c.UserContext(), userID, &query)
	if err != nil {
		return MapError(err)
	}

	return c.JSON(result)
}

// PUT /api/v1/tasks/:id
func (h *TaskHandler) Update(c *fiber.Ctx) error {
	userID, ok := middleware.UserID(c)
	if !ok {
		return fiber.ErrUnauthorized
	}

	taskID := c.Params("id")
	if taskID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "task ID is required")
	}

	var req domain.UpdateTaskRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	if err := h.validator.Validate(req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	task, err := h.svc.UpdateTask(c.UserContext(), userID, taskID, &req)
	if err != nil {
		return MapError(err)
	}

	return c.JSON(task)
}

// GET /api/v1/tasks/:id/history
func (h *TaskHandler) GetHistory(c *fiber.Ctx) error {
	userID, ok := middleware.UserID(c)
	if !ok {
		return fiber.ErrUnauthorized
	}

	taskID := c.Params("id")
	if taskID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "task ID is required")
	}

	history, err := h.svc.GetTaskHistory(c.UserContext(), userID, taskID)
	if err != nil {
		return MapError(err)
	}

	return c.JSON(history)
}

// POST /api/v1/tasks/:id/comments
func (h *TaskHandler) AddComment(c *fiber.Ctx) error {
	userID, ok := middleware.UserID(c)
	if !ok {
		return fiber.ErrUnauthorized
	}

	taskID := c.Params("id")
	if taskID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "task ID is required")
	}

	var req domain.CreateCommentRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	if err := h.validator.Validate(req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	comment, err := h.svc.AddComment(c.UserContext(), userID, taskID, &req)
	if err != nil {
		return MapError(err)
	}

	return c.Status(fiber.StatusCreated).JSON(comment)
}

// GET /api/v1/tasks/:id/comments
func (h *TaskHandler) ListComments(c *fiber.Ctx) error {
	userID, ok := middleware.UserID(c)
	if !ok {
		return fiber.ErrUnauthorized
	}

	taskID := c.Params("id")
	if taskID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "task ID is required")
	}

	comments, err := h.svc.ListComments(c.UserContext(), userID, taskID)
	if err != nil {
		return MapError(err)
	}

	return c.JSON(comments)
}
