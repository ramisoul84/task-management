package handler

import (
	"errors"
	"net/http"

	"github.com/gofiber/fiber/v2"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/server/http/middleware"
	"github.com/ramisoul84/task-management/pkg/logger"
	"github.com/ramisoul84/task-management/pkg/validator"
)

// ErrorResponse is the body of every non-2xx response.
//
// "error" stays a plain string for backward compatibility with existing
// clients; "code" is the stable machine-readable value to switch on.
type ErrorResponse struct {
	Error     string                 `json:"error" example:"insufficient permissions"`
	Code      domain.ErrorCode       `json:"code" example:"forbidden"`
	Details   []validator.FieldError `json:"details,omitempty"`
	RequestID string                 `json:"request_id,omitempty" example:"0f8c2a4e-1b7d-4a53-9a41-5bd1f2a1c0de"`
}

// MessageResponse is a generic success body.
type MessageResponse struct {
	Message string `json:"message" example:"ok"`
}

var codeStatus = map[domain.ErrorCode]int{
	domain.CodeInvalidInput:       http.StatusBadRequest,
	domain.CodeValidationFailed:   http.StatusBadRequest,
	domain.CodeAssigneeNotInTeam:  http.StatusBadRequest,
	domain.CodeUnauthorized:       http.StatusUnauthorized,
	domain.CodeInvalidCredentials: http.StatusUnauthorized,
	domain.CodeForbidden:          http.StatusForbidden,
	domain.CodeNotFound:           http.StatusNotFound,
	domain.CodeAlreadyExists:      http.StatusConflict,
	domain.CodeConflict:           http.StatusConflict,
	domain.CodeVersionConflict:    http.StatusConflict,
	domain.CodeTooManyRequests:    http.StatusTooManyRequests,
	domain.CodeUnavailable:        http.StatusServiceUnavailable,
	domain.CodeInternal:           http.StatusInternalServerError,
}

var statusCode = map[int]domain.ErrorCode{
	http.StatusBadRequest:          domain.CodeInvalidInput,
	http.StatusUnauthorized:        domain.CodeUnauthorized,
	http.StatusForbidden:           domain.CodeForbidden,
	http.StatusNotFound:            domain.CodeNotFound,
	http.StatusConflict:            domain.CodeConflict,
	http.StatusTooManyRequests:     domain.CodeTooManyRequests,
	http.StatusInternalServerError: domain.CodeInternal,
}

// ErrorHandler renders any error returned by a handler or middleware as an
// ErrorResponse. It is the single place errors become HTTP responses, so
// handlers just `return err`.
//
// Domain error messages are written for clients and always safe to show;
// their wrapped causes (SQL, Redis, ...) are logged for 5xx and never sent.
// Any other error becomes a generic 500.
func ErrorHandler(c *fiber.Ctx, err error) error {
	status, body := toResponse(err)
	body.RequestID = middleware.GetRequestID(c)

	if status >= http.StatusInternalServerError {
		// 503s are expected during a dependency outage (one per request
		// while the breaker is open); keep them out of the error stream.
		l := logger.FromContext(c.UserContext())
		ev := l.Error()
		if status == http.StatusServiceUnavailable {
			ev = l.Warn()
		}
		ev.Err(err).Str("method", c.Method()).Str("path", c.Path()).Msg("request error")
	}

	switch status {
	case http.StatusUnauthorized:
		c.Set(fiber.HeaderWWWAuthenticate, `Bearer realm="api"`)
	case http.StatusServiceUnavailable:
		c.Set(fiber.HeaderRetryAfter, "5")
	}

	return c.Status(status).JSON(body)
}

func toResponse(err error) (int, ErrorResponse) {
	if verr, ok := errors.AsType[validator.Errors](err); ok {
		return http.StatusBadRequest, ErrorResponse{
			Error:   "request validation failed",
			Code:    domain.CodeValidationFailed,
			Details: verr.Fields,
		}
	}

	if derr, ok := errors.AsType[*domain.Error](err); ok {
		status, known := codeStatus[derr.Code]
		if !known {
			return http.StatusInternalServerError, ErrorResponse{Error: domain.ErrInternal.Message, Code: domain.CodeInternal}
		}
		return status, ErrorResponse{Error: derr.Message, Code: derr.Code}
	}

	// Errors raised by Fiber itself: unknown route, body too large, ...
	if ferr, ok := errors.AsType[*fiber.Error](err); ok && ferr.Code < http.StatusInternalServerError {
		code, known := statusCode[ferr.Code]
		if !known {
			code = domain.CodeInvalidInput
		}
		return ferr.Code, ErrorResponse{Error: ferr.Message, Code: code}
	}

	return http.StatusInternalServerError, ErrorResponse{Error: domain.ErrInternal.Message, Code: domain.CodeInternal}
}

// bindJSON decodes the JSON body into dst and validates it.
func bindJSON(c *fiber.Ctx, v *validator.Validator, dst any) error {
	if err := c.BodyParser(dst); err != nil {
		return domain.ErrInvalidInput.WithMessage("request body must be valid JSON")
	}
	return v.Validate(dst)
}

// bindQuery decodes query parameters into dst and validates it.
func bindQuery(c *fiber.Ctx, v *validator.Validator, dst any) error {
	if err := c.QueryParser(dst); err != nil {
		return domain.ErrInvalidInput.WithMessage("invalid query parameters")
	}
	return v.Validate(dst)
}

// pathUUID reads a path parameter and checks that it is a UUID, so
// malformed IDs fail fast with 400 instead of a pointless DB round-trip.
func pathUUID(c *fiber.Ctx, v *validator.Validator, name string) (string, error) {
	id := c.Params(name)
	if !v.IsUUID(id) {
		return "", domain.ErrInvalidInput.WithMessage(name + " must be a valid UUID")
	}
	return id, nil
}

// currentUser returns the authenticated user ID set by the auth middleware.
func currentUser(c *fiber.Ctx) (string, error) {
	id, ok := middleware.UserID(c)
	if !ok || id == "" {
		return "", domain.ErrUnauthorized
	}
	return id, nil
}
