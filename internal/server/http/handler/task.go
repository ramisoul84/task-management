package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/service"
	"github.com/ramisoul84/task-management/pkg/validator"
)

type TaskHandler struct {
	svc service.TaskService
	v   *validator.Validator
}

func NewTaskHandler(svc service.TaskService, v *validator.Validator) *TaskHandler {
	return &TaskHandler{svc: svc, v: v}
}

// Create godoc
//
//	@Summary		Create a task
//	@Description	Any team member can create a task. The caller is the creator; the assignee defaults to the caller and must belong to the team.
//	@Tags			Tasks
//	@Security		BearerAuth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		domain.CreateTaskRequest	true	"Task"
//	@Success		201		{object}	domain.Task
//	@Failure		400		{object}	ErrorResponse	"Validation error / assignee not in team"
//	@Failure		401		{object}	ErrorResponse
//	@Failure		403		{object}	ErrorResponse	"Not a team member"
//	@Failure		500		{object}	ErrorResponse
//	@Router			/tasks [post]
func (h *TaskHandler) Create(c *fiber.Ctx) error {
	userID, err := currentUser(c)
	if err != nil {
		return err
	}

	var req domain.CreateTaskRequest
	if err := bindJSON(c, h.v, &req); err != nil {
		return err
	}

	task, err := h.svc.CreateTask(c.UserContext(), userID, &req)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(task)
}

// ListByTeam godoc
//
//	@Summary		List a team's tasks
//	@Description	Filtered, paginated list (newest first). Results are cached in Redis for 5 minutes per team+filter combination and invalidated on any task change.
//	@Tags			Tasks
//	@Security		BearerAuth
//	@Produce		json
//	@Param			team_id		query		string	true	"Team ID"	format(uuid)
//	@Param			status		query		string	false	"Status"	Enums(todo, in_progress, review, done, cancelled)
//	@Param			assignee_id	query		string	false	"Assignee"	format(uuid)
//	@Param			limit		query		int		false	"Page size (1-100)"	default(20)
//	@Param			offset		query		int		false	"Offset"			default(0)
//	@Success		200			{object}	domain.TaskListResponse
//	@Failure		400			{object}	ErrorResponse
//	@Failure		401			{object}	ErrorResponse
//	@Failure		403			{object}	ErrorResponse	"Not a team member"
//	@Router			/tasks [get]
func (h *TaskHandler) ListByTeam(c *fiber.Ctx) error {
	userID, err := currentUser(c)
	if err != nil {
		return err
	}

	var q domain.TaskListQuery
	if err := bindQuery(c, h.v, &q); err != nil {
		return err
	}

	res, err := h.svc.ListTasks(c.UserContext(), userID, &q)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Get godoc
//
//	@Summary	Get a task
//	@Tags		Tasks
//	@Security	BearerAuth
//	@Produce	json
//	@Param		id	path		string	true	"Task ID"	format(uuid)
//	@Success	200	{object}	domain.Task
//	@Failure	400	{object}	ErrorResponse
//	@Failure	401	{object}	ErrorResponse
//	@Failure	404	{object}	ErrorResponse	"Task not found"
//	@Router		/tasks/{id} [get]
func (h *TaskHandler) Get(c *fiber.Ctx) error {
	userID, err := currentUser(c)
	if err != nil {
		return err
	}
	taskID, err := pathUUID(c, h.v, "id")
	if err != nil {
		return err
	}

	task, err := h.svc.GetTask(c.UserContext(), userID, taskID)
	if err != nil {
		return err
	}
	return c.JSON(task)
}

// Delete godoc
//
//	@Summary		Delete a task
//	@Description	Permanently deletes the task with its history and comments. Owner, admin or the task's creator only.
//	@Tags			Tasks
//	@Security		BearerAuth
//	@Param			id	path	string	true	"Task ID"	format(uuid)
//	@Success		204
//	@Failure		400	{object}	ErrorResponse
//	@Failure		401	{object}	ErrorResponse
//	@Failure		403	{object}	ErrorResponse
//	@Failure		404	{object}	ErrorResponse	"Task not found"
//	@Router			/tasks/{id} [delete]
func (h *TaskHandler) Delete(c *fiber.Ctx) error {
	userID, err := currentUser(c)
	if err != nil {
		return err
	}
	taskID, err := pathUUID(c, h.v, "id")
	if err != nil {
		return err
	}

	if err := h.svc.DeleteTask(c.UserContext(), userID, taskID); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// Update godoc
//
//	@Summary		Update a task
//	@Description	Partial update guarded by optimistic locking: send the version you read; a stale version returns 409. Owner/admin/creator may change any field; the assignee may change only the status. Every change is recorded in history with old and new values.
//	@Tags			Tasks
//	@Security		BearerAuth
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string						true	"Task ID"	format(uuid)
//	@Param			request	body		domain.UpdateTaskRequest	true	"Fields to change"
//	@Success		200		{object}	domain.Task
//	@Failure		400		{object}	ErrorResponse	"Validation error / assignee not in team"
//	@Failure		401		{object}	ErrorResponse
//	@Failure		403		{object}	ErrorResponse	"Not allowed to change this field"
//	@Failure		404		{object}	ErrorResponse	"Task not found"
//	@Failure		409		{object}	ErrorResponse	"Version conflict"
//	@Router			/tasks/{id} [put]
func (h *TaskHandler) Update(c *fiber.Ctx) error {
	userID, err := currentUser(c)
	if err != nil {
		return err
	}
	taskID, err := pathUUID(c, h.v, "id")
	if err != nil {
		return err
	}

	var req domain.UpdateTaskRequest
	if err := bindJSON(c, h.v, &req); err != nil {
		return err
	}

	task, err := h.svc.UpdateTask(c.UserContext(), userID, taskID, &req)
	if err != nil {
		return err
	}
	return c.JSON(task)
}

// GetHistory godoc
//
//	@Summary	Task change history
//	@Tags		Tasks
//	@Security	BearerAuth
//	@Produce	json
//	@Param		id	path		string	true	"Task ID"	format(uuid)
//	@Success	200	{array}		domain.TaskHistory
//	@Failure	400	{object}	ErrorResponse
//	@Failure	401	{object}	ErrorResponse
//	@Failure	404	{object}	ErrorResponse	"Task not found"
//	@Router		/tasks/{id}/history [get]
func (h *TaskHandler) GetHistory(c *fiber.Ctx) error {
	userID, err := currentUser(c)
	if err != nil {
		return err
	}
	taskID, err := pathUUID(c, h.v, "id")
	if err != nil {
		return err
	}

	history, err := h.svc.GetHistory(c.UserContext(), userID, taskID)
	if err != nil {
		return err
	}
	return c.JSON(history)
}

// AddComment godoc
//
//	@Summary	Comment on a task
//	@Tags		Tasks
//	@Security	BearerAuth
//	@Accept		json
//	@Produce	json
//	@Param		id		path		string						true	"Task ID"	format(uuid)
//	@Param		request	body		domain.CreateCommentRequest	true	"Comment"
//	@Success	201		{object}	domain.TaskComment
//	@Failure	400		{object}	ErrorResponse
//	@Failure	401		{object}	ErrorResponse
//	@Failure	404		{object}	ErrorResponse	"Task not found"
//	@Router		/tasks/{id}/comments [post]
func (h *TaskHandler) AddComment(c *fiber.Ctx) error {
	userID, err := currentUser(c)
	if err != nil {
		return err
	}
	taskID, err := pathUUID(c, h.v, "id")
	if err != nil {
		return err
	}

	var req domain.CreateCommentRequest
	if err := bindJSON(c, h.v, &req); err != nil {
		return err
	}

	comment, err := h.svc.AddComment(c.UserContext(), userID, taskID, &req)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(comment)
}

// ListComments godoc
//
//	@Summary	List task comments
//	@Tags		Tasks
//	@Security	BearerAuth
//	@Produce	json
//	@Param		id	path		string	true	"Task ID"	format(uuid)
//	@Success	200	{array}		domain.TaskComment
//	@Failure	400	{object}	ErrorResponse
//	@Failure	401	{object}	ErrorResponse
//	@Failure	404	{object}	ErrorResponse	"Task not found"
//	@Router		/tasks/{id}/comments [get]
func (h *TaskHandler) ListComments(c *fiber.Ctx) error {
	userID, err := currentUser(c)
	if err != nil {
		return err
	}
	taskID, err := pathUUID(c, h.v, "id")
	if err != nil {
		return err
	}

	comments, err := h.svc.ListComments(c.UserContext(), userID, taskID)
	if err != nil {
		return err
	}
	return c.JSON(comments)
}
