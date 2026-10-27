package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// decodeStrictObject splits one JSON object into the raw value of each field.
// It rejects a field name that is not in allowed, compared with exact case, and
// a field name that repeats. The standard decoder matches names without regard
// to case and keeps the last of a repeated name, so a typo or a repeat could
// change behavior without any error.
func decodeStrictObject(data []byte, allowed ...string) (map[string]json.RawMessage, error) {
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
			// Cap the quoted name, because it is untrusted and can be as long as the body.
			return nil, fmt.Errorf("unknown field %.64q, field names are case sensitive and must be one of %s", name, strings.Join(allowed, ", "))
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
	// More also reports false at the end of the input, so only Token notices a
	// body that ends before the closing brace.
	_, err = decoder.Token()
	return fields, err
}

// unmarshalField decodes the field called name into target when the object
// has it, and names the field in the error.
func unmarshalField(fields map[string]json.RawMessage, name string, target any) error {
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
