package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/server/http/handler"
	"github.com/ramisoul84/task-management/internal/server/http/middleware"
	"github.com/ramisoul84/task-management/pkg/logger"
	"github.com/ramisoul84/task-management/pkg/validator"
)

// app mirrors the production middleware order around a handler that
// returns err (or panics when err is nil).
func app(t *testing.T, err error) *fiber.App {
	t.Helper()
	a := fiber.New(fiber.Config{ErrorHandler: handler.ErrorHandler})
	a.Use(middleware.RequestID(logger.Nop()), middleware.AccessLog(), middleware.Recover())
	a.All("/x", func(c *fiber.Ctx) error {
		if err == nil {
			panic("kaboom")
		}
		return err
	})
	return a
}

type errBody struct {
	Error     string `json:"error"`
	Code      string `json:"code"`
	RequestID string `json:"request_id"`
	Details   []struct {
		Field, Message string
	} `json:"details"`
}

func call(t *testing.T, a *fiber.App, method, path, body string) (int, errBody, http.Header, string) {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var b errBody
	_ = json.Unmarshal(raw, &b)
	return resp.StatusCode, b, resp.Header, string(raw)
}

func TestErrorHandler_DomainErrors(t *testing.T) {
	tests := []struct {
		err        error
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{domain.ErrForbidden.WithMessage("nope"), 403, "forbidden", "nope"},
		{domain.ErrNotFound, 404, "not_found", "resource not found"},
		{domain.ErrVersionConflict, 409, "version_conflict", domain.ErrVersionConflict.Message},
		{domain.ErrAssigneeNotInTeam, 400, "assignee_not_in_team", domain.ErrAssigneeNotInTeam.Message},
		{domain.ErrInvalidCredentials, 401, "invalid_credentials", "invalid credentials"},
		{domain.ErrTooManyRequests, 429, "too_many_requests", "too many requests"},
		{domain.ErrUnavailable.Wrap(errors.New("redis: dial tcp: refused")), 503, "service_unavailable", domain.ErrUnavailable.Message},
		// wrapped further up the stack
		{errors.Join(errors.New("ctx"), domain.ErrForbidden), 403, "forbidden", "insufficient permissions"},
	}
	for _, tc := range tests {
		t.Run(tc.wantCode, func(t *testing.T) {
			status, body, hdr, raw := call(t, app(t, tc.err), http.MethodGet, "/x", "")
			if status != tc.wantStatus || body.Code != tc.wantCode || body.Error != tc.wantMsg {
				t.Fatalf("got %d %q %q, want %d %q %q", status, body.Code, body.Error, tc.wantStatus, tc.wantCode, tc.wantMsg)
			}
			if body.RequestID == "" || body.RequestID != hdr.Get(middleware.HeaderRequestID) {
				t.Fatalf("request_id %q must match the X-Request-ID header %q", body.RequestID, hdr.Get(middleware.HeaderRequestID))
			}
			if strings.Contains(raw, "refused") {
				t.Fatalf("internal cause leaked to client: %s", raw)
			}
		})
	}
}

func TestErrorHandler_UnknownErrorsAreOpaque(t *testing.T) {
	status, body, _, raw := call(t, app(t, errors.New("sql: connection refused at 10.0.0.5")), http.MethodGet, "/x", "")
	if status != 500 || body.Code != "internal_error" || strings.Contains(raw, "10.0.0.5") {
		t.Fatalf("got %d %s", status, raw)
	}
}

func TestErrorHandler_ValidationDetails(t *testing.T) {
	type req struct {
		Title string `json:"title" validate:"required,min=3"`
		Email string `json:"email" validate:"required,email"`
	}
	err := validator.New().Validate(req{Title: "ab", Email: "nope"})

	status, body, _, _ := call(t, app(t, err), http.MethodPost, "/x", "")
	if status != 400 || body.Code != "validation_failed" || len(body.Details) != 2 {
		t.Fatalf("got %d %+v", status, body)
	}
	if body.Details[0].Field != "title" || body.Details[1].Field != "email" {
		t.Fatalf("details must use JSON field names: %+v", body.Details)
	}
}

func TestErrorHandler_PanicBecomesJSON500(t *testing.T) {
	status, body, _, raw := call(t, app(t, nil), http.MethodGet, "/x", "")
	if status != 500 || body.Code != "internal_error" || strings.Contains(raw, "kaboom") {
		t.Fatalf("got %d %s", status, raw)
	}
}

func TestErrorHandler_FrameworkErrors(t *testing.T) {
	a := app(t, domain.ErrNotFound)

	status, body, _, _ := call(t, a, http.MethodGet, "/does-not-exist", "")
	if status != 404 || body.Code != "not_found" {
		t.Fatalf("unknown route: %d %+v", status, body)
	}
	// Body-size limits are enforced by fasthttp while reading the
	// connection, before any handler runs, so they are not testable here.
}

func TestRequestID_RejectsUnsafeIncomingIDs(t *testing.T) {
	a := app(t, domain.ErrNotFound)

	for _, tc := range []struct {
		incoming string
		kept     bool
	}{
		{"abc-123", true},
		{"evil\nfake log line", false},
		{strings.Repeat("x", 100), false},
	} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/x", http.NoBody)
		req.Header.Set(middleware.HeaderRequestID, tc.incoming)
		resp, err := a.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		got := resp.Header.Get(middleware.HeaderRequestID)
		if (got == tc.incoming) != tc.kept || got == "" {
			t.Errorf("incoming %q -> %q (kept=%v)", tc.incoming, got, tc.kept)
		}
	}
}
