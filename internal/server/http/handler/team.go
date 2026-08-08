package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/service"
	"github.com/ramisoul84/task-management/pkg/validator"
)

type TeamHandler struct {
	svc service.TeamService
	v   *validator.Validator
}

func NewTeamHandler(svc service.TeamService, v *validator.Validator) *TeamHandler {
	return &TeamHandler{svc: svc, v: v}
}

// Create godoc
//
//	@Summary		Create a team
//	@Description	Creates a team; the caller becomes its owner. Team and owner membership are created atomically.
//	@Tags			Teams
//	@Security		BearerAuth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		domain.CreateTeamRequest	true	"Team"
//	@Success		201		{object}	domain.Team
//	@Failure		400		{object}	ErrorResponse	"Validation error"
//	@Failure		401		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/teams [post]
func (h *TeamHandler) Create(c *fiber.Ctx) error {
	userID, err := currentUser(c)
	if err != nil {
		return err
	}

	var req domain.CreateTeamRequest
	if err := bindJSON(c, h.v, &req); err != nil {
		return err
	}

	team, err := h.svc.CreateTeam(c.UserContext(), userID, req.Name)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(team)
}

// List godoc
//
//	@Summary		List my teams
//	@Description	Teams the caller belongs to, with the caller's role in each.
//	@Tags			Teams
//	@Security		BearerAuth
//	@Produce		json
//	@Success		200	{array}		domain.Team
//	@Failure		401	{object}	ErrorResponse
//	@Failure		500	{object}	ErrorResponse
//	@Router			/teams [get]
func (h *TeamHandler) List(c *fiber.Ctx) error {
	userID, err := currentUser(c)
	if err != nil {
		return err
	}

	teams, err := h.svc.ListTeams(c.UserContext(), userID)
	if err != nil {
		return err
	}
	return c.JSON(teams)
}

// Invite godoc
//
//	@Summary		Invite a member
//	@Description	Adds an existing user to the team as a member. Owner or admin only.
//	@Tags			Teams
//	@Security		BearerAuth
//	@Accept			json
//	@Param			id		path	string						true	"Team ID"	format(uuid)
//	@Param			request	body	domain.InviteMemberRequest	true	"Who to invite"
//	@Success		204
//	@Failure		400	{object}	ErrorResponse	"Validation error"
//	@Failure		401	{object}	ErrorResponse
//	@Failure		403	{object}	ErrorResponse	"Not a team manager"
//	@Failure		404	{object}	ErrorResponse	"User not found"
//	@Failure		409	{object}	ErrorResponse	"Already a member"
//	@Router			/teams/{id}/invite [post]
func (h *TeamHandler) Invite(c *fiber.Ctx) error {
	userID, err := currentUser(c)
	if err != nil {
		return err
	}
	teamID, err := pathUUID(c, h.v, "id")
	if err != nil {
		return err
	}

	var req domain.InviteMemberRequest
	if err := bindJSON(c, h.v, &req); err != nil {
		return err
	}

	if err := h.svc.InviteMember(c.UserContext(), userID, teamID, &req); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ListMembers godoc
//
//	@Summary	List team members
//	@Tags		Teams
//	@Security	BearerAuth
//	@Produce	json
//	@Param		id	path		string	true	"Team ID"	format(uuid)
//	@Success	200	{array}		domain.TeamMemberResponse
//	@Failure	400	{object}	ErrorResponse
//	@Failure	401	{object}	ErrorResponse
//	@Failure	403	{object}	ErrorResponse	"Not a team member"
//	@Router		/teams/{id}/members [get]
func (h *TeamHandler) ListMembers(c *fiber.Ctx) error {
	userID, err := currentUser(c)
	if err != nil {
		return err
	}
	teamID, err := pathUUID(c, h.v, "id")
	if err != nil {
		return err
	}

	members, err := h.svc.ListMembers(c.UserContext(), userID, teamID)
	if err != nil {
		return err
	}
	return c.JSON(members)
}

// ChangeMemberRole godoc
//
//	@Summary		Change a member's role
//	@Description	Owner can change admins and members; admins can change members only. The owner's role never changes.
//	@Tags			Teams
//	@Security		BearerAuth
//	@Accept			json
//	@Param			id		path	string							true	"Team ID"	format(uuid)
//	@Param			userID	path	string							true	"User ID"	format(uuid)
//	@Param			request	body	domain.ChangeMemberRoleRequest	true	"New role"
//	@Success		204
//	@Failure		400	{object}	ErrorResponse
//	@Failure		401	{object}	ErrorResponse
//	@Failure		403	{object}	ErrorResponse
//	@Failure		404	{object}	ErrorResponse	"Member not found"
//	@Router			/teams/{id}/members/{userID}/role [patch]
func (h *TeamHandler) ChangeMemberRole(c *fiber.Ctx) error {
	userID, err := currentUser(c)
	if err != nil {
		return err
	}
	teamID, err := pathUUID(c, h.v, "id")
	if err != nil {
		return err
	}
	targetID, err := pathUUID(c, h.v, "userID")
	if err != nil {
		return err
	}

	var req domain.ChangeMemberRoleRequest
	if err := bindJSON(c, h.v, &req); err != nil {
		return err
	}

	if err := h.svc.ChangeMemberRole(c.UserContext(), userID, teamID, targetID, req.Role); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// RemoveMember godoc
//
//	@Summary		Remove a member
//	@Description	Removes a member and unassigns their tasks in this team (recorded in task history). The owner cannot be removed.
//	@Tags			Teams
//	@Security		BearerAuth
//	@Param			id		path	string	true	"Team ID"	format(uuid)
//	@Param			userID	path	string	true	"User ID"	format(uuid)
//	@Success		204
//	@Failure		400	{object}	ErrorResponse
//	@Failure		401	{object}	ErrorResponse
//	@Failure		403	{object}	ErrorResponse
//	@Failure		404	{object}	ErrorResponse	"Member not found"
//	@Router			/teams/{id}/members/{userID} [delete]
func (h *TeamHandler) RemoveMember(c *fiber.Ctx) error {
	userID, err := currentUser(c)
	if err != nil {
		return err
	}
	teamID, err := pathUUID(c, h.v, "id")
	if err != nil {
		return err
	}
	targetID, err := pathUUID(c, h.v, "userID")
	if err != nil {
		return err
	}

	if err := h.svc.RemoveMember(c.UserContext(), userID, teamID, targetID); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// GetStats godoc
//
//	@Summary		Team analytics
//	@Description	Tasks by status, top 3 assignees by tasks closed in the last 30 days, average close time and comment count. Owner or admin only.
//	@Tags			Teams
//	@Security		BearerAuth
//	@Produce		json
//	@Param			id	path		string	true	"Team ID"	format(uuid)
//	@Success		200	{object}	domain.TeamStats
//	@Failure		400	{object}	ErrorResponse
//	@Failure		401	{object}	ErrorResponse
//	@Failure		403	{object}	ErrorResponse	"Not owner/admin"
//	@Router			/teams/{id}/stats [get]
func (h *TeamHandler) GetStats(c *fiber.Ctx) error {
	userID, err := currentUser(c)
	if err != nil {
		return err
	}
	teamID, err := pathUUID(c, h.v, "id")
	if err != nil {
		return err
	}

	stats, err := h.svc.GetStats(c.UserContext(), userID, teamID)
	if err != nil {
		return err
	}
	return c.JSON(stats)
}
