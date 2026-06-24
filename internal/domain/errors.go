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
)
