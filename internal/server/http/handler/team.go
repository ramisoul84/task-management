package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/server/http/middleware"
	"github.com/ramisoul84/task-management/internal/service"
	"github.com/ramisoul84/task-management/pkg/validator"
)

type TeamHandler struct {
	svc       service.TeamService
	validator *validator.Validator
}

func NewTeamHandler(svc service.TeamService, validator *validator.Validator) *TeamHandler {
	return &TeamHandler{
		svc:       svc,
		validator: validator,
	}
}

func (h *TeamHandler) Create(c *fiber.Ctx) error {
	userID, ok := middleware.UserID(c)
	if !ok {
		return fiber.ErrUnauthorized
	}

	var req domain.CreateTeamRequest

	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(
			fiber.StatusBadRequest,
			"invalid request body",
		)
	}

	if err := h.validator.Validate(req); err != nil {
		return fiber.NewError(
			fiber.StatusBadRequest,
			err.Error(),
		)
	}

	team, err := h.svc.CreateTeam(c.UserContext(), userID, req.Name)
	if err != nil {
		return MapError(err)
	}

	return c.Status(fiber.StatusCreated).JSON(team)
}

func (h *TeamHandler) List(c *fiber.Ctx) error {
	userID, ok := middleware.UserID(c)
	if !ok {
		return fiber.ErrUnauthorized
	}

	teams, err := h.svc.ListTeamsByUser(c.UserContext(), userID)
	if err != nil {
		return MapError(err)
	}

	if teams == nil {
		teams = []*domain.Team{}
	}

	return c.JSON(teams)
}

// POST /api/v1/teams/:id/invite
func (h *TeamHandler) Invite(c *fiber.Ctx) error {
	actorID, ok := middleware.UserID(c)
	if !ok {
		return fiber.ErrUnauthorized
	}

	teamID := c.Params("id")

	var req domain.InviteMemberRequest

	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(
			fiber.StatusBadRequest,
			"invalid request body",
		)
	}

	if err := h.validator.Validate(req); err != nil {
		return fiber.NewError(
			fiber.StatusBadRequest,
			err.Error(),
		)
	}

	if err := h.svc.InviteMember(c.UserContext(), actorID, teamID, req.UserID); err != nil {
		return MapError(err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// GET /api/v1/teams/:id/members
func (h *TeamHandler) ListMembers(c *fiber.Ctx) error {
	userID, ok := middleware.UserID(c)
	if !ok {
		return fiber.ErrUnauthorized
	}

	teamID := c.Params("id")
	if teamID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "team ID is required")
	}

	members, err := h.svc.ListMembers(c.UserContext(), userID, teamID)
	if err != nil {
		return MapError(err)
	}

	return c.JSON(members)
}

// PATCH /api/v1/teams/:id/members/:userID/role
func (h *TeamHandler) ChangeMemberRole(c *fiber.Ctx) error {
	actorID, ok := middleware.UserID(c)
	if !ok {
		return fiber.ErrUnauthorized
	}

	teamID := c.Params("id")
	if teamID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "team ID is required")
	}

	targetUserID := c.Params("userID")
	if targetUserID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "user ID is required")
	}

	var req domain.ChangeMemberRoleRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	if err := h.validator.Validate(req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	if err := h.svc.ChangeMemberRole(c.UserContext(), actorID, teamID, targetUserID, req.Role); err != nil {
		return MapError(err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// DELETE /api/v1/teams/:id/members/:userID
func (h *TeamHandler) RemoveMember(c *fiber.Ctx) error {
	actorID, ok := middleware.UserID(c)
	if !ok {
		return fiber.ErrUnauthorized
	}

	teamID := c.Params("id")
	if teamID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "team ID is required")
	}

	targetUserID := c.Params("userID")
	if targetUserID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "user ID is required")
	}

	if err := h.svc.RemoveMember(c.UserContext(), actorID, teamID, targetUserID); err != nil {
		return MapError(err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}
