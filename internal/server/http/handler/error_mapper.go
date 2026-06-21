package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/ramisoul84/task-management/internal/domain"
)

// MapError maps domain errors to HTTP errors
func MapError(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, "resource not found")

	case errors.Is(err, domain.ErrUnauthorized):
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")

	case errors.Is(err, domain.ErrInvalidCredentials):
		return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")

	case errors.Is(err, domain.ErrInvalidInput):
		return fiber.NewError(fiber.StatusBadRequest, "invalid input")

	case errors.Is(err, domain.ErrAlreadyExists):
		return fiber.NewError(fiber.StatusConflict, "resource already exists")

	default:
		return fiber.NewError(fiber.StatusInternalServerError, "internal server error")
	}
}
