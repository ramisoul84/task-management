package handler

import (
	"strings"

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

// Register godoc
// @Summary Register new user
// @Description Create a new user account with email and password
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body domain.RegisterRequest true "Registration details"
// @Success 201 {object} domain.UserResponse "User registered successfully"
// @Failure 400 {object} domain.ErrorResponse "Validation error"
// @Failure 409 {object} domain.ErrorResponse "Email already exists"
// @Failure 500 {object} domain.ErrorResponse "Internal server error"
// @Router /auth/register [post]
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
		return MapError(err)
	}

	return c.Status(fiber.StatusCreated).JSON(domain.UserResponse{
		ID:    user.ID,
		Email: user.Email,
		Name:  user.Name,
	})
}

// Login godoc
// @Summary Login user
// @Description Authenticate user with email and password
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body domain.LoginRequest true "Login credentials"
// @Success 200 {object} domain.LoginResponse "Login successful"
// @Failure 400 {object} domain.ErrorResponse "Validation error"
// @Failure 401 {object} domain.ErrorResponse "Invalid credentials"
// @Failure 500 {object} domain.ErrorResponse "Internal server error"
// @Router /auth/login [post]
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
		return MapError(err)
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

// Refresh godoc
// @Summary Refresh access token
// @Description Get new access token using refresh token
// @Tags Auth
// @Accept json
// @Produce json
// @Param refresh_token body string true "Refresh token"
// @Success 200 {object} domain.RefreshResponse "Token refreshed"
// @Failure 400 {object} domain.ErrorResponse "Validation error"
// @Failure 401 {object} domain.ErrorResponse "Invalid refresh token"
// @Failure 500 {object} domain.ErrorResponse "Internal server error"
// @Router /auth/refresh [post]
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
		return MapError(err)
	}

	h.setRefreshCookie(c, result.RefreshToken)
	return c.JSON(domain.RefreshResponse{
		AccessToken: result.AccessToken,
		TokenType:   "Bearer",
		ExpiresIn:   result.ExpiresIn,
	})
}

// Logout godoc
// @Summary Logout user
// @Description Revoke access token and refresh token
// @Tags Auth
// @Security BearerAuth
// @Success 204 "Logged out successfully"
// @Failure 401 {object} domain.ErrorResponse "Unauthorized"
// @Failure 500 {object} domain.ErrorResponse "Internal server error"
// @Router /auth/logout [post]
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	refreshToken := c.Cookies(refreshCookieName)

	accessToken := ""
	if header := c.Get(fiber.HeaderAuthorization); strings.HasPrefix(header, "Bearer ") {
		accessToken = strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	}

	if err := h.svc.Logout(c.UserContext(), accessToken, refreshToken); err != nil {
		return MapError(err)
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
