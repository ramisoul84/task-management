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

// Create godoc
// @Summary Create task
// @Description Create a new task in a team
// @Tags Tasks
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body domain.CreateTaskRequest true "Task details"
// @Success 201 {object} domain.Task "Task created"
// @Failure 400 {object} domain.ErrorResponse "Validation error"
// @Failure 401 {object} domain.ErrorResponse "Unauthorized"
// @Failure 403 {object} domain.ErrorResponse "Not team member"
// @Failure 500 {object} domain.ErrorResponse "Internal server error"
// @Router /tasks [post]
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

// ListByTeam godoc
// @Summary List tasks
// @Description Get tasks with filters and pagination
// @Tags Tasks
// @Security BearerAuth
// @Produce json
// @Param team_id query string true "Team ID"
// @Param status query string false "Task status" Enums(todo, in_progress, review, done, cancelled)
// @Param assignee_id query string false "Assignee ID"
// @Param limit query int false "Items per page" default(20)
// @Param offset query int false "Offset" default(0)
// @Success 200 {object} domain.TaskListResponse "Tasks retrieved"
// @Failure 400 {object} domain.ErrorResponse "Validation error"
// @Failure 401 {object} domain.ErrorResponse "Unauthorized"
// @Failure 403 {object} domain.ErrorResponse "Not team member"
// @Router /tasks [get]
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

// Update godoc
// @Summary Update task
// @Description Update task with permission checks
// @Tags Tasks
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Task ID"
// @Param request body domain.UpdateTaskRequest true "Task updates"
// @Success 200 {object} domain.Task "Task updated"
// @Failure 400 {object} domain.ErrorResponse "Validation error"
// @Failure 401 {object} domain.ErrorResponse "Unauthorized"
// @Failure 403 {object} domain.ErrorResponse "Forbidden"
// @Failure 404 {object} domain.ErrorResponse "Task not found"
// @Failure 409 {object} domain.ErrorResponse "Version conflict"
// @Router /tasks/{id} [put]
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

// GetHistory godoc
// @Summary Get task history
// @Description Get change history for a task
// @Tags Tasks
// @Security BearerAuth
// @Produce json
// @Param id path string true "Task ID"
// @Success 200 {array} domain.TaskHistory "Task history"
// @Failure 401 {object} domain.ErrorResponse "Unauthorized"
// @Failure 403 {object} domain.ErrorResponse "Forbidden"
// @Failure 404 {object} domain.ErrorResponse "Task not found"
// @Router /tasks/{id}/history [get]
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

// AddComment godoc
// @Summary Add comment
// @Description Add comment to a task
// @Tags Tasks
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Task ID"
// @Param request body domain.CreateCommentRequest true "Comment content"
// @Success 201 {object} domain.TaskComment "Comment added"
// @Failure 400 {object} domain.ErrorResponse "Validation error"
// @Failure 401 {object} domain.ErrorResponse "Unauthorized"
// @Failure 403 {object} domain.ErrorResponse "Forbidden"
// @Failure 404 {object} domain.ErrorResponse "Task not found"
// @Router /tasks/{id}/comments [post]
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

// ListComments godoc
// @Summary List comments
// @Description Get all comments for a task
// @Tags Tasks
// @Security BearerAuth
// @Produce json
// @Param id path string true "Task ID"
// @Success 200 {array} domain.TaskComment "Comments retrieved"
// @Failure 401 {object} domain.ErrorResponse "Unauthorized"
// @Failure 403 {object} domain.ErrorResponse "Forbidden"
// @Failure 404 {object} domain.ErrorResponse "Task not found"
// @Router /tasks/{id}/comments [get]
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
