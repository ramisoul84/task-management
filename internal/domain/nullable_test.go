package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ramisoul84/task-management/internal/domain"
)

func TestNullable_DistinguishesOmittedNullAndValue(t *testing.T) {
	type body struct {
		Due domain.Nullable[time.Time] `json:"due_at"`
	}

	tests := []struct {
		name      string
		in        string
		set, ok   bool
		wantValue string
	}{
		{"omitted", `{}`, false, false, ""},
		{"null", `{"due_at": null}`, true, false, ""},
		{"value", `{"due_at": "2030-05-01T10:00:00Z"}`, true, true, "2030-05-01T10:00:00Z"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var b body
			if err := json.Unmarshal([]byte(tc.in), &b); err != nil {
				t.Fatal(err)
			}
			if b.Due.Set != tc.set || b.Due.Valid != tc.ok {
				t.Fatalf("Set=%v Valid=%v, want %v %v", b.Due.Set, b.Due.Valid, tc.set, tc.ok)
			}
			if tc.ok && b.Due.Value.Format(time.RFC3339) != tc.wantValue {
				t.Fatalf("value = %v", b.Due.Value)
			}
			if !tc.ok && b.Due.Ptr() != nil {
				t.Fatal("Ptr must be nil when not valid")
			}
		})
	}

	var b body
	if err := json.Unmarshal([]byte(`{"due_at": "not-a-time"}`), &b); err == nil {
		t.Fatal("invalid value must fail to decode")
	}
}
