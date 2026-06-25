package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/ramisoul84/task-management/internal/domain"
)

// MapError maps domain errors to HTTP errors
func MapError(err error) error {
	switch {
	// 404 Not Found
	case errors.Is(err, domain.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, "resource not found")

	// 401 Unauthorized
	case errors.Is(err, domain.ErrUnauthorized):
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")

	case errors.Is(err, domain.ErrInvalidCredentials):
		return fiber.NewError(fiber.StatusUnauthorized, "invalid email or password")

	// 403 Forbidden
	case errors.Is(err, domain.ErrForbidden):
		return fiber.NewError(fiber.StatusForbidden, "insufficient permissions")

	// 400 Bad Request
	case errors.Is(err, domain.ErrInvalidInput):
		return fiber.NewError(fiber.StatusBadRequest, "invalid input")

	// 409 Conflict
	case errors.Is(err, domain.ErrAlreadyExists):
		return fiber.NewError(fiber.StatusConflict, "resource already exists")

	case errors.Is(err, domain.ErrConflict):
		return fiber.NewError(fiber.StatusConflict, "resource conflict")

	// 429 Too Many Requests
	case errors.Is(err, domain.ErrTooManyRequests):
		return fiber.NewError(fiber.StatusTooManyRequests, "too many requests")

	// 500 Internal Server Error
	default:
		return fiber.NewError(fiber.StatusInternalServerError, "internal server error")
	}
}
