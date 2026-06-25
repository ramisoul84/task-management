// internal/domain/errors.go
package domain

import "errors"

var (
	// ErrInvalidInput - input validation failed
	ErrInvalidInput = errors.New("invalid input")

	// ErrNotFound - resource not found in database
	ErrNotFound = errors.New("not found")

	// ErrAlreadyExists - resource already exists
	ErrAlreadyExists = errors.New("already exists")

	// ErrInvalidCredentials - wrong email or password
	ErrInvalidCredentials = errors.New("invalid credentials")

	// ErrUnauthorized - user is not authenticated
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden - user doesn't have permission to perform action
	ErrForbidden = errors.New("forbidden")

	// ErrConflict - resource conflict (e.g., version mismatch)
	ErrConflict = errors.New("conflict")

	// ErrInternalServer - internal server error
	ErrInternalServer = errors.New("internal server error")

	// ErrTooManyRequests - rate limit exceeded
	ErrTooManyRequests = errors.New("too many requests")

	// ErrAssigneeNotInTeam - assignee is not a member of the team
	ErrAssigneeNotInTeam = errors.New("assignee not in team")
)
