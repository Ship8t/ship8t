// Package strictjson decodes JSON objects for the manifest and the config files.
// It rejects what the standard decoder lets through, which is a field name with
// the wrong case and a repeated field name. It also rejects data after the
// object, which a streaming decoder leaves unread.
package strictjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

// UnknownFieldError reports a field name that an object does not accept.
type UnknownFieldError struct {
	Name    string
	Allowed []string
}

// Error lists the accepted field names, and caps the quoted name because it is
// untrusted and can be as long as the input.
func (err *UnknownFieldError) Error() string {
	return fmt.Sprintf("unknown field %.64q, field names are case sensitive and must be one of %s", err.Name, strings.Join(err.Allowed, ", "))
}

// DecodeObject splits one JSON object into the raw value of each field. It
// rejects a field name that is not in allowed, compared with exact case, a field
// name that repeats, and any data after the object. The standard decoder
// matches names without regard to case and keeps the last of a repeated name, so
// a typo or a repeat could change a setting without any error.
func DecodeObject(data []byte, allowed ...string) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))

	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if token != json.Delim('{') {
		return nil, errors.New("want a JSON object")
	}

	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		// The key of a JSON object is always a string.
		name, _ := token.(string)
		if !slices.Contains(allowed, name) {
			return nil, &UnknownFieldError{Name: name, Allowed: allowed}
		}
		if _, repeated := fields[name]; repeated {
			return nil, fmt.Errorf("field %q appears more than once", name)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		fields[name] = value
	}
	// On some Go versions, More reports false for an input that ends before the
	// closing brace, so only Token notices it. Go 1.26 does this for a bare {.
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("unexpected data after the object")
	}
	return fields, nil
}

// UnmarshalField decodes the field called name into target when the object has
// it, and names the field in the error. Like the standard decoder, it accepts
// null and leaves the target as it was.
func UnmarshalField(fields map[string]json.RawMessage, name string, target any) error {
	raw, found := fields[name]
	if !found {
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		// Cap the text, because a type error can hold the whole untrusted number.
		// Nothing needs the wrapped error, so the chain is dropped.
		return fmt.Errorf("%s: %.100s", name, err)
	}
	return nil
}

// IsNull reports whether a raw JSON value is null.
func IsNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}
