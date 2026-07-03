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

// Create godoc
// @Summary Create team
// @Description Create a new team with current user as owner
// @Tags Teams
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body domain.CreateTeamRequest true "Team details"
// @Success 201 {object} domain.Team "Team created successfully"
// @Failure 400 {object} domain.ErrorResponse "Validation error"
// @Failure 401 {object} domain.ErrorResponse "Unauthorized"
// @Failure 500 {object} domain.ErrorResponse "Internal server error"
// @Router /teams [post]
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

// List godoc
// @Summary List user's teams
// @Description Get all teams where current user is a member
// @Tags Teams
// @Security BearerAuth
// @Produce json
// @Success 200 {array} domain.Team "Teams retrieved"
// @Failure 401 {object} domain.ErrorResponse "Unauthorized"
// @Failure 500 {object} domain.ErrorResponse "Internal server error"
// @Router /teams [get]
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

// Invite godoc
// @Summary Invite member
// @Description Invite user to team (owner/admin only)
// @Tags Teams
// @Security BearerAuth
// @Accept json
// @Param id path string true "Team ID"
// @Param request body domain.InviteMemberRequest true "User to invite"
// @Success 204 "Member invited"
// @Failure 400 {object} domain.ErrorResponse "Validation error"
// @Failure 401 {object} domain.ErrorResponse "Unauthorized"
// @Failure 403 {object} domain.ErrorResponse "Forbidden"
// @Failure 404 {object} domain.ErrorResponse "User not found"
// @Failure 409 {object} domain.ErrorResponse "Already a member"
// @Router /teams/{id}/invite [post]
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

// ListMembers godoc
// @Summary List team members
// @Description Get all members of a team
// @Tags Teams
// @Security BearerAuth
// @Produce json
// @Param id path string true "Team ID"
// @Success 200 {array} domain.TeamMemberResponse "Members retrieved"
// @Failure 401 {object} domain.ErrorResponse "Unauthorized"
// @Failure 403 {object} domain.ErrorResponse "Forbidden"
// @Failure 404 {object} domain.ErrorResponse "Team not found"
// @Router /teams/{id}/members [get]
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

// ChangeMemberRole godoc
// @Summary Change member role
// @Description Change user's role in team (owner/admin only)
// @Tags Teams
// @Security BearerAuth
// @Accept json
// @Param id path string true "Team ID"
// @Param userID path string true "User ID"
// @Param request body domain.ChangeMemberRoleRequest true "New role"
// @Success 204 "Role updated"
// @Failure 400 {object} domain.ErrorResponse "Validation error"
// @Failure 401 {object} domain.ErrorResponse "Unauthorized"
// @Failure 403 {object} domain.ErrorResponse "Forbidden"
// @Failure 404 {object} domain.ErrorResponse "User not found"
// @Router /teams/{id}/members/{userID}/role [patch]
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

// RemoveMember godoc
// @Summary Remove member
// @Description Remove user from team (owner/admin only)
// @Tags Teams
// @Security BearerAuth
// @Param id path string true "Team ID"
// @Param userID path string true "User ID"
// @Success 204 "Member removed"
// @Failure 401 {object} domain.ErrorResponse "Unauthorized"
// @Failure 403 {object} domain.ErrorResponse "Forbidden"
// @Failure 404 {object} domain.ErrorResponse "User not found"
// @Router /teams/{id}/members/{userID} [delete]
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

// GetStats godoc
// @Summary Get team statistics
// @Description Get analytics for team (owner/admin only)
// @Tags Teams
// @Security BearerAuth
// @Produce json
// @Param id path string true "Team ID"
// @Success 200 {object} domain.TeamStats "Team statistics"
// @Failure 401 {object} domain.ErrorResponse "Unauthorized"
// @Failure 403 {object} domain.ErrorResponse "Forbidden"
// @Failure 404 {object} domain.ErrorResponse "Team not found"
// @Router /teams/{id}/stats [get]
func (h *TeamHandler) GetStats(c *fiber.Ctx) error {
	userID, ok := middleware.UserID(c)
	if !ok {
		return fiber.ErrUnauthorized
	}

	teamID := c.Params("id")
	if teamID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "team ID is required")
	}

	stats, err := h.svc.GetTeamStats(c.UserContext(), userID, teamID)
	if err != nil {
		return MapError(err)
	}

	return c.JSON(stats)
}
