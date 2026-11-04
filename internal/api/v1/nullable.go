package v1

import (
	"bytes"
	"encoding/json"
)

// nullable decodes a JSON property while remembering whether it was present and
// whether it was an explicit null. This distinguishes "leave unchanged" (absent)
// from "clear the value" (null) in PATCH bodies.
type nullable[T any] struct {
	Set   bool
	Null  bool
	Value T
}

// UnmarshalJSON records presence and nullness; json calls it for explicit nulls
// on non-pointer fields, but never for absent properties.
func (n *nullable[T]) UnmarshalJSON(data []byte) error {
	n.Set = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		n.Null = true
		var zero T
		n.Value = zero
		return nil
	}
	n.Null = false
	return json.Unmarshal(data, &n.Value)
}

// applyTo updates dst: absent leaves it unchanged, null clears it, and a
// concrete value replaces it.
func (n nullable[T]) applyTo(dst **T) {
	switch {
	case !n.Set:
	case n.Null:
		*dst = nil
	default:
		v := n.Value
		*dst = &v
	}
}

// pointer returns nil for absent and null values, otherwise a pointer to the value.
func (n nullable[T]) pointer() *T {
	if !n.Set || n.Null {
		return nil
	}
	v := n.Value
	return &v
}
