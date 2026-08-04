package domain

import "errors"

// ErrorCode is a stable, machine-readable identifier clients can switch on.
type ErrorCode string

const (
	CodeInvalidInput       ErrorCode = "invalid_input"
	CodeValidationFailed   ErrorCode = "validation_failed"
	CodeNotFound           ErrorCode = "not_found"
	CodeAlreadyExists      ErrorCode = "already_exists"
	CodeConflict           ErrorCode = "conflict"
	CodeVersionConflict    ErrorCode = "version_conflict"
	CodeUnauthorized       ErrorCode = "unauthorized"
	CodeInvalidCredentials ErrorCode = "invalid_credentials" //nolint:gosec // error code, not a credential
	CodeForbidden          ErrorCode = "forbidden"
	CodeAssigneeNotInTeam  ErrorCode = "assignee_not_in_team"
	CodeTooManyRequests    ErrorCode = "too_many_requests"
	CodeInternal           ErrorCode = "internal_error"
)

// Error is a business error. Message is safe to show to API clients; Err
// (optional) carries the underlying cause for logs only.
//
// Two *Error values match under errors.Is when their codes are equal, so
// callers can return a more specific message while still matching the
// package-level sentinels:
//
//	return domain.ErrForbidden.WithMessage("only the owner can delete a team")
//	errors.Is(err, domain.ErrForbidden) // true
type Error struct {
	Code    ErrorCode
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Err }

func (e *Error) Is(target error) bool {
	if t, ok := errors.AsType[*Error](target); ok {
		return t.Code == e.Code
	}
	return false
}

// WithMessage returns a copy of e with a more specific client message.
func (e *Error) WithMessage(msg string) *Error {
	return &Error{Code: e.Code, Message: msg, Err: e.Err}
}

// Wrap returns a copy of e that records cause for logging.
func (e *Error) Wrap(cause error) *Error {
	return &Error{Code: e.Code, Message: e.Message, Err: cause}
}

var (
	ErrInvalidInput       = &Error{Code: CodeInvalidInput, Message: "invalid input"}
	ErrNotFound           = &Error{Code: CodeNotFound, Message: "resource not found"}
	ErrAlreadyExists      = &Error{Code: CodeAlreadyExists, Message: "resource already exists"}
	ErrConflict           = &Error{Code: CodeConflict, Message: "resource has been modified by another request"}
	ErrVersionConflict    = &Error{Code: CodeVersionConflict, Message: "task version is outdated; reload and retry"}
	ErrUnauthorized       = &Error{Code: CodeUnauthorized, Message: "authentication required"}
	ErrInvalidCredentials = &Error{Code: CodeInvalidCredentials, Message: "invalid credentials"}
	ErrForbidden          = &Error{Code: CodeForbidden, Message: "insufficient permissions"}
	ErrAssigneeNotInTeam  = &Error{Code: CodeAssigneeNotInTeam, Message: "assignee must be a member of the team"}
	ErrTooManyRequests    = &Error{Code: CodeTooManyRequests, Message: "too many requests"}
	ErrInternal           = &Error{Code: CodeInternal, Message: "internal server error"}
)
