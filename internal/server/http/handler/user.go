package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/service"
	"github.com/ramisoul84/task-management/pkg/validator"
)

type UserHandler struct {
	svc service.UserService
	v   *validator.Validator
}

func NewUserHandler(svc service.UserService, v *validator.Validator) *UserHandler {
	return &UserHandler{svc: svc, v: v}
}

// Me godoc
//
//	@Summary	Current user
//	@Tags		Users
//	@Security	BearerAuth
//	@Produce	json
//	@Success	200	{object}	domain.UserResponse
//	@Failure	401	{object}	ErrorResponse
//	@Router		/users/me [get]
func (h *UserHandler) Me(c *fiber.Ctx) error {
	userID, err := currentUser(c)
	if err != nil {
		return err
	}

	user, err := h.svc.Me(c.UserContext(), userID)
	if err != nil {
		return err
	}
	return c.JSON(user.Response())
}

// UpdateProfile godoc
//
//	@Summary	Update my profile
//	@Tags		Users
//	@Security	BearerAuth
//	@Accept		json
//	@Produce	json
//	@Param		request	body		domain.UpdateProfileRequest	true	"Profile"
//	@Success	200		{object}	domain.UserResponse
//	@Failure	400		{object}	ErrorResponse
//	@Failure	401		{object}	ErrorResponse
//	@Router		/users/me [patch]
func (h *UserHandler) UpdateProfile(c *fiber.Ctx) error {
	userID, err := currentUser(c)
	if err != nil {
		return err
	}

	var req domain.UpdateProfileRequest
	if err := bindJSON(c, h.v, &req); err != nil {
		return err
	}

	user, err := h.svc.UpdateProfile(c.UserContext(), userID, &req)
	if err != nil {
		return err
	}
	return c.JSON(user.Response())
}

// ChangePassword godoc
//
//	@Summary		Change my password
//	@Description	Requires the current password.
//	@Tags			Users
//	@Security		BearerAuth
//	@Accept			json
//	@Param			request	body	domain.ChangePasswordRequest	true	"Passwords"
//	@Success		204
//	@Failure		400	{object}	ErrorResponse
//	@Failure		401	{object}	ErrorResponse	"Wrong current password"
//	@Router			/users/me/password [put]
func (h *UserHandler) ChangePassword(c *fiber.Ctx) error {
	userID, err := currentUser(c)
	if err != nil {
		return err
	}

	var req domain.ChangePasswordRequest
	if err := bindJSON(c, h.v, &req); err != nil {
		return err
	}

	if err := h.svc.ChangePassword(c.UserContext(), userID, &req); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
