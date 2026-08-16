package handler

import (
	"errors"

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

func NewTeamHandler(
	svc service.TeamService,
	validator *validator.Validator,
) *TeamHandler {
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
		return mapTeamError(err)
	}

	return c.Status(fiber.StatusCreated).JSON(team)
}

func (h *TeamHandler) List(c *fiber.Ctx) error {
	userID, ok := middleware.UserID(c)
	if !ok {
		return fiber.ErrUnauthorized
	}

	teams, err := h.svc.ListTeams(c.UserContext(), userID)
	if err != nil {
		return mapTeamError(err)
	}

	return c.JSON(teams)
}

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
		return mapTeamError(err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *TeamHandler) ChangeMemberRole(c *fiber.Ctx) error {
	actorID, ok := middleware.UserID(c)
	if !ok {
		return fiber.ErrUnauthorized
	}

	teamID := c.Params("id")
	targetUserID := c.Params("userID")

	var req domain.ChangeMemberRoleRequest

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

	if err := h.svc.ChangeMemberRole(
		c.UserContext(),
		actorID,
		teamID,
		targetUserID,
		req.Role,
	); err != nil {
		return mapTeamError(err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *TeamHandler) RemoveMember(c *fiber.Ctx) error {
	actorID, ok := middleware.UserID(c)
	if !ok {
		return fiber.ErrUnauthorized
	}

	teamID := c.Params("id")
	targetUserID := c.Params("userID")

	if err := h.svc.RemoveMember(
		c.UserContext(),
		actorID,
		teamID,
		targetUserID,
	); err != nil {
		return mapTeamError(err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func mapTeamError(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		return fiber.NewError(
			fiber.StatusBadRequest,
			"invalid request",
		)

	case errors.Is(err, domain.ErrNotFound):
		return fiber.NewError(
			fiber.StatusNotFound,
			"team or member not found",
		)

	case errors.Is(err, domain.ErrAlreadyExists):
		return fiber.NewError(
			fiber.StatusConflict,
			"member already exists",
		)

	case errors.Is(err, domain.ErrForbidden):
		return fiber.NewError(
			fiber.StatusForbidden,
			"you do not have permission to perform this action",
		)

	default:
		return fiber.NewError(
			fiber.StatusInternalServerError,
			"internal server error",
		)
	}
}
