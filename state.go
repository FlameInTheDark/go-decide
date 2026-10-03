package decide

import (
	"encoding/json"
	"fmt"
	"strings"
)

// State is the content a decision model evaluates. It is either a non-empty
// string, a JSON object or a JSON array of related context.
//
// Build one with [Text], [Object], [List] or [JSON]. The zero State is invalid;
// requests containing it fail validation.
type State struct {
	value any
}

// Text returns a State holding a plain string.
func Text(s string) State {
	return State{value: s}
}

// Object returns a State holding a JSON object.
func Object(obj map[string]any) State {
	return State{value: obj}
}

// List returns a State holding a JSON array.
func List(items []any) State {
	return State{value: items}
}

// JSON returns a State built from an arbitrary value. Strings, maps, slices,
// structs and anything else that marshals to a JSON object or array are
// accepted; values that marshal to a bare JSON scalar are rejected because the
// System One endpoints require a string, object or array.
func JSON(v any) (State, error) {
	switch v.(type) {
	case nil:
		return State{}, fmt.Errorf("decide: state must not be nil")
	}

	raw, err := json.Marshal(v)
	if err != nil {
		return State{}, fmt.Errorf("decide: encode state: %w", err)
	}

	switch firstToken(raw) {
	case '{', '[':
		return State{value: v}, nil
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return State{}, fmt.Errorf("decide: decode state string: %w", err)
		}
		return State{value: s}, nil
	default:
		return State{}, fmt.Errorf("decide: state must marshal to a string, object or array, got %s", kindOf(raw))
	}
}

// firstToken reports the first meaningful byte of an encoded JSON document.
func firstToken(raw []byte) byte {
	for _, b := range raw {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			return b
		}
	}
	return 0
}

func kindOf(raw []byte) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "nothing"
	}
	if trimmed == "null" {
		return "null"
	}
	if trimmed == "true" || trimmed == "false" {
		return "boolean"
	}
	return "number"
}

// IsZero reports whether the State carries no value and is therefore invalid.
func (s State) IsZero() bool {
	switch v := s.value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	default:
		return false
	}
}

// MarshalJSON implements [json.Marshaler].
func (s State) MarshalJSON() ([]byte, error) {
	if s.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(s.value)
}

// UnmarshalJSON implements [json.Unmarshaler]. JSON numbers are decoded as
// float64, matching encoding/json defaults.
func (s *State) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*s = State{}
		return nil
	}

	switch trimmed[0] {
	case '"':
		var str string
		if err := json.Unmarshal(data, &str); err != nil {
			return err
		}
		*s = Text(str)
		return nil
	case '{':
		var obj map[string]any
		if err := json.Unmarshal(data, &obj); err != nil {
			return err
		}
		*s = Object(obj)
		return nil
	case '[':
		var items []any
		if err := json.Unmarshal(data, &items); err != nil {
			return err
		}
		*s = List(items)
		return nil
	default:
		return fmt.Errorf("decide: state must be a string, object or array, got %s", kindOf(data))
	}
}

// String renders the state as JSON, which is handy for logging and tests.
func (s State) String() string {
	if s.IsZero() {
		return ""
	}
	raw, err := json.Marshal(s.value)
	if err != nil {
		return ""
	}
	return string(raw)
}
