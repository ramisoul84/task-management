package domain

import (
	"bytes"
	"encoding/json"
)

// Nullable distinguishes the three states a PATCH-style JSON field can be
// in, which a plain pointer cannot:
//
//	field omitted        -> Set=false               (leave unchanged)
//	"field": null        -> Set=true,  Valid=false  (clear it)
//	"field": <value>     -> Set=true,  Valid=true   (set it)
type Nullable[T any] struct {
	Set   bool
	Valid bool
	Value T
}

// UnmarshalJSON is only invoked when the key is present, which is what
// makes Set meaningful.
func (n *Nullable[T]) UnmarshalJSON(b []byte) error {
	n.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		var zero T
		n.Valid, n.Value = false, zero
		return nil
	}
	if err := json.Unmarshal(b, &n.Value); err != nil {
		return err
	}
	n.Valid = true
	return nil
}

func (n Nullable[T]) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(n.Value)
}

// Ptr returns the value as a pointer (nil when null or unset).
func (n Nullable[T]) Ptr() *T {
	if !n.Valid {
		return nil
	}
	v := n.Value
	return &v
}
