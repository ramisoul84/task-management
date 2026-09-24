package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramisoul84/task-management/internal/config"
	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/server/http/middleware"
	"github.com/ramisoul84/task-management/internal/service"
	"github.com/ramisoul84/task-management/pkg/validator"
)

const (
	refreshCookieName = "refresh_token"
	refreshCookiePath = "/api/v1/auth"
)

type AuthHandler struct {
	svc service.AuthService
	v   *validator.Validator
	cfg config.AuthConfig
}

func NewAuthHandler(svc service.AuthService, v *validator.Validator, cfg config.AuthConfig) *AuthHandler {
	return &AuthHandler{svc: svc, v: v, cfg: cfg}
}

// Register godoc
//
//	@Summary		Register a new user
//	@Description	Creates an account. Email is case-insensitive and must be unique. Also served at /auth/register.
//	@Tags			Auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		domain.RegisterRequest	true	"Registration details"
//	@Success		201		{object}	domain.UserResponse
//	@Failure		400		{object}	ErrorResponse	"Validation error"
//	@Failure		409		{object}	ErrorResponse	"Email already registered"
//	@Failure		429		{object}	ErrorResponse	"Rate limit exceeded"
//	@Failure		500		{object}	ErrorResponse
//	@Router			/register [post]
func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req domain.RegisterRequest
	if err := bindJSON(c, h.v, &req); err != nil {
		return err
	}

	user, err := h.svc.Register(c.UserContext(), &req)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(user.Response())
}

// Login godoc
//
//	@Summary		Log in
//	@Description	Returns a short-lived JWT access token and sets an HttpOnly refresh-token cookie (path /api/v1/auth). Also served at /auth/login.
//	@Tags			Auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		domain.LoginRequest	true	"Credentials"
//	@Success		200		{object}	domain.LoginResponse
//	@Header			200		{string}	Set-Cookie		"refresh_token=...; HttpOnly"
//	@Failure		400		{object}	ErrorResponse	"Validation error"
//	@Failure		401		{object}	ErrorResponse	"Invalid credentials"
//	@Failure		429		{object}	ErrorResponse	"Rate limit exceeded"
//	@Failure		500		{object}	ErrorResponse
//	@Router			/login [post]
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req domain.LoginRequest
	if err := bindJSON(c, h.v, &req); err != nil {
		return err
	}

	res, err := h.svc.Login(c.UserContext(), &req)
	if err != nil {
		return err
	}

	h.setRefreshCookie(c, res.Token.RefreshToken)

	return c.JSON(domain.LoginResponse{
		User:        res.User.Response(),
		AccessToken: res.Token.AccessToken,
		TokenType:   "Bearer",
		ExpiresIn:   res.Token.ExpiresIn,
	})
}

// Refresh godoc
//
//	@Summary		Refresh the access token
//	@Description	Exchanges the refresh-token cookie for a new access token and rotates the cookie. Each refresh token works once; replaying an old one fails.
//	@Tags			Auth
//	@Produce		json
//	@Param			Cookie	header		string	true	"refresh_token=<token>"
//	@Success		200		{object}	domain.RefreshResponse
//	@Failure		401		{object}	ErrorResponse	"Missing, expired or reused refresh token"
//	@Failure		500		{object}	ErrorResponse
//	@Router			/auth/refresh [post]
func (h *AuthHandler) Refresh(c *fiber.Ctx) error {
	pair, err := h.svc.Refresh(c.UserContext(), c.Cookies(refreshCookieName))
	if err != nil {
		h.clearRefreshCookie(c)
		return err
	}

	h.setRefreshCookie(c, pair.RefreshToken)

	return c.JSON(domain.RefreshResponse{
		AccessToken: pair.AccessToken,
		TokenType:   "Bearer",
		ExpiresIn:   pair.ExpiresIn,
	})
}

// Logout godoc
//
//	@Summary		Log out
//	@Description	Revokes the bearer access token (if sent) and the refresh-token cookie. Idempotent.
//	@Tags			Auth
//	@Security		BearerAuth
//	@Success		204
//	@Failure		500	{object}	ErrorResponse
//	@Router			/auth/logout [post]
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	access, _ := middleware.BearerToken(c)

	if err := h.svc.Logout(c.UserContext(), access, c.Cookies(refreshCookieName)); err != nil {
		return err
	}

	h.clearRefreshCookie(c)
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *AuthHandler) setRefreshCookie(c *fiber.Ctx, token string) {
	c.Cookie(&fiber.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     refreshCookiePath,
		Domain:   h.cfg.CookieDomain,
		MaxAge:   int(h.cfg.RefreshTokenTTL.Seconds()),
		HTTPOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: h.cfg.CookieSameSite,
	})
}

func (h *AuthHandler) clearRefreshCookie(c *fiber.Ctx) {
	c.Cookie(&fiber.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     refreshCookiePath,
		Domain:   h.cfg.CookieDomain,
		MaxAge:   -1,
		HTTPOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: h.cfg.CookieSameSite,
	})
}
