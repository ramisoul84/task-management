package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/ramisoul84/task-management/internal/config"
	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/service"
	"github.com/ramisoul84/task-management/pkg/validator"
)

const (
	refreshCookieName = "refresh_token"
	refreshCookiePath = "/api/v1/auth"
)

type AuthHandler struct {
	svc       service.AuthService
	validator *validator.Validator
	cfg       config.SecurityConfig
}

func NewAuthHandler(svc service.AuthService, validator *validator.Validator, cfg config.SecurityConfig) *AuthHandler {
	return &AuthHandler{
		svc:       svc,
		validator: validator,
		cfg:       cfg,
	}
}

func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req domain.RegisterRequest

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

	user, err := h.svc.Register(c.UserContext(), req.Email, req.Password, req.Name)
	if err != nil {
		return handleAuthError(err)
	}

	return c.Status(fiber.StatusCreated).JSON(domain.UserResponse{
		ID:    user.ID,
		Email: user.Email,
		Name:  user.Name,
	})
}

func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req domain.LoginRequest

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

	result, err := h.svc.Login(c.UserContext(), req.Email, req.Password)
	if err != nil {
		return handleAuthError(err)
	}

	h.setRefreshCookie(c, result.Token.RefreshToken)

	return c.JSON(domain.LoginResponse{
		User: domain.UserResponse{
			ID:    result.User.ID,
			Email: result.User.Email,
			Name:  result.User.Name,
		},
		AccessToken: result.Token.AccessToken,
		TokenType:   "Bearer",
		ExpiresIn:   result.Token.ExpiresIn,
	})
}

func (h *AuthHandler) Refresh(c *fiber.Ctx) error {
	refreshToken := c.Cookies(refreshCookieName)

	if refreshToken == "" {
		return fiber.NewError(
			fiber.StatusUnauthorized,
			"refresh token required",
		)
	}

	result, err := h.svc.Refresh(c.UserContext(), refreshToken)
	if err != nil {
		return handleAuthError(err)
	}

	h.setRefreshCookie(c, result.RefreshToken)
	return c.JSON(domain.RefreshResponse{
		AccessToken: result.AccessToken,
		TokenType:   "Bearer",
		ExpiresIn:   result.ExpiresIn,
	})
}

func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	refreshToken := c.Cookies(refreshCookieName)

	if refreshToken != "" {
		if err := h.svc.Logout(c.UserContext(), refreshToken); err != nil {
			return handleAuthError(err)
		}
	}

	h.clearRefreshCookie(c)

	return c.SendStatus(fiber.StatusNoContent)
}

// setRefreshCookie sets the refresh cookie
func (h *AuthHandler) setRefreshCookie(c *fiber.Ctx, token string) {
	c.Cookie(&fiber.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     refreshCookiePath,
		Domain:   h.cfg.CookieDomain,
		HTTPOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: h.cfg.CookieSameSite,
		MaxAge:   int(h.cfg.RefreshTokenTTL.Seconds()),
	})
}

func (h *AuthHandler) clearRefreshCookie(c *fiber.Ctx) {
	c.Cookie(&fiber.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     refreshCookiePath,
		HTTPOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: h.cfg.CookieSameSite,
		MaxAge:   -1,
	})
}

// handleAuthError handles auth errors
func handleAuthError(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidCredentials):
		return fiber.NewError(
			fiber.StatusUnauthorized,
			"invalid credentials",
		)

	case errors.Is(err, domain.ErrAlreadyExists):
		return fiber.NewError(
			fiber.StatusConflict,
			"email already exists",
		)

	default:
		return fiber.NewError(
			fiber.StatusInternalServerError,
			"internal server error",
		)
	}
}
