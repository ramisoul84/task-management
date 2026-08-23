package domain

import (
	"encoding/base64"
	"encoding/json"
	"time"
)

// TaskCursor marks a position in a task listing for keyset pagination:
// "the rows after (Time, ID) in this sort order". Unlike OFFSET it stays
// fast on deep pages and never skips or repeats rows when tasks are added
// or removed between requests.
type TaskCursor struct {
	Sort  string    `json:"s"`
	Order string    `json:"o"`
	Time  time.Time `json:"t"`
	ID    string    `json:"i"`
}

// SupportsCursor reports whether a sort field can be keyset-paginated.
// Only non-null, time-based sorts qualify.
func SupportsCursor(sort string) bool {
	return sort == "" || sort == SortCreatedAt || sort == SortUpdatedAt
}

// CursorAfter returns the cursor pointing just past task.
func CursorAfter(task *Task, sort, order string) TaskCursor {
	c := TaskCursor{Sort: sortOrDefault(sort), Order: orderOrDefault(order), ID: task.ID, Time: task.CreatedAt}
	if c.Sort == SortUpdatedAt {
		c.Time = task.UpdatedAt
	}
	return c
}

// Encode returns the opaque string clients pass back as ?cursor=.
func (c TaskCursor) Encode() string {
	raw, _ := json.Marshal(c) // fixed struct of strings and a time: cannot fail
	return base64.RawURLEncoding.EncodeToString(raw)
}

// DecodeTaskCursor parses a cursor and checks it belongs to a listing with
// the same sort and order; a cursor from a different ordering would
// silently return the wrong rows.
func DecodeTaskCursor(s, sort, order string) (TaskCursor, error) {
	invalid := ErrInvalidInput.WithMessage("cursor is invalid or expired")

	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return TaskCursor{}, invalid
	}
	var c TaskCursor
	if err := json.Unmarshal(raw, &c); err != nil || c.ID == "" || c.Time.IsZero() {
		return TaskCursor{}, invalid
	}
	if c.Sort != sortOrDefault(sort) || c.Order != orderOrDefault(order) {
		return TaskCursor{}, ErrInvalidInput.WithMessage("cursor was issued for a different sort order")
	}
	return c, nil
}

func sortOrDefault(s string) string {
	if s == "" {
		return SortCreatedAt
	}
	return s
}

func orderOrDefault(o string) string {
	if o == "" {
		return "desc"
	}
	return o
}
