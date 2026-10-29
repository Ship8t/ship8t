package strictjson

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestDecodeObjectAcceptsObjects(t *testing.T) {
	tests := []struct {
		name string
		data string
		want map[string]string
	}{
		{"empty object", `{}`, map[string]string{}},
		{"surrounding whitespace", " \n\t{ }\n ", map[string]string{}},
		{"one field", `{"a":1}`, map[string]string{"a": `1`}},
		{"fields in any order", `{"b":"x","a":true}`, map[string]string{"a": `true`, "b": `"x"`}},
		{"nested values stay raw", `{"a":{"b":[1,2]},"b":null}`, map[string]string{"a": `{"b":[1,2]}`, "b": `null`}},
		{"name written with an escape", `{"\u0061":1}`, map[string]string{"a": `1`}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fields, err := DecodeObject([]byte(test.data), "a", "b")
			if err != nil {
				t.Fatalf("DecodeObject(%q) error = %v", test.data, err)
			}
			got := make(map[string]string, len(fields))
			for name, raw := range fields {
				got[name] = string(raw)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("DecodeObject(%q) = %v, want %v", test.data, got, test.want)
			}
		})
	}
}

func TestDecodeObjectRejectsInvalidObjects(t *testing.T) {
	// truncated marks the inputs that end too early. The error must say so, and
	// must not blame data after the object.
	tests := []struct {
		name      string
		data      string
		wantInErr string
		truncated bool
	}{
		{"unknown field", `{"c":1}`, "unknown field", false},
		{"unknown field after a valid one", `{"a":1,"c":1}`, "unknown field", false},
		{"field with the wrong case", `{"A":1}`, "case sensitive", false},
		{"repeated field", `{"a":1,"a":2}`, `field "a" appears more than once`, false},
		{"repeated field with other fields between", `{"a":1,"b":2,"a":3}`, "appears more than once", false},

		// The caller may pass any bytes, so malformed input must fail and not panic.
		{"empty input", ``, "", true},
		{"open brace only", `{`, "", true},
		{"key without a value", `{"a"`, "", true},
		{"key with a colon only", `{"a":`, "", true},
		{"value without a closing brace", `{"a":1`, "", true},
		{"value that is not JSON", `{"a":x}`, "", false},
		{"array", `[]`, "want a JSON object", false},
		{"null", `null`, "want a JSON object", false},
		{"number", `5`, "want a JSON object", false},
		{"string", `"a"`, "want a JSON object", false},

		{"second object", `{} {}`, "unexpected data after the object", false},
		{"text after the object", `{"a":1} x`, "unexpected data after the object", false},
		{"array after the object", `{} []`, "unexpected data after the object", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fields, err := DecodeObject([]byte(test.data), "a", "b")
			if err == nil {
				t.Fatalf("DecodeObject(%q) = %v, nil error, want an error", test.data, fields)
			}
			if fields != nil {
				t.Errorf("DecodeObject(%q) = %v with an error, want nil fields", test.data, fields)
			}
			if !strings.Contains(err.Error(), test.wantInErr) {
				t.Errorf("DecodeObject(%q) error = %q, want it to contain %q", test.data, err, test.wantInErr)
			}
			if test.truncated && strings.Contains(err.Error(), "unexpected data") {
				t.Errorf("DecodeObject(%q) error = %q, want the input reported as cut short, not as data after the object", test.data, err)
			}
		})
	}
}

func TestDecodeObjectReportsTheUnknownFieldAsAType(t *testing.T) {
	_, err := DecodeObject([]byte(`{"Secret":1}`), "a", "b")

	var unknown *UnknownFieldError
	if !errors.As(err, &unknown) {
		t.Fatalf("DecodeObject() error = %v, want an *UnknownFieldError", err)
	}
	if unknown.Name != "Secret" {
		t.Errorf("UnknownFieldError.Name = %q, want %q", unknown.Name, "Secret")
	}
	if !reflect.DeepEqual(unknown.Allowed, []string{"a", "b"}) {
		t.Errorf("UnknownFieldError.Allowed = %v, want [a b]", unknown.Allowed)
	}
	want := `unknown field "Secret", field names are case sensitive and must be one of a, b`
	if err.Error() != want {
		t.Errorf("UnknownFieldError text = %q, want %q", err, want)
	}
}

func TestDecodeObjectCapsTheUnknownFieldName(t *testing.T) {
	data := `{"` + strings.Repeat("x", 60000) + `":1}`

	_, err := DecodeObject([]byte(data), "a", "b")
	if err == nil {
		t.Fatal("DecodeObject() = nil error, want an error")
	}
	if len(err.Error()) > 200 {
		t.Errorf("DecodeObject() error is %d bytes, want the quoted name capped", len(err.Error()))
	}
	// The cap is 64 characters, no more and no fewer.
	if !strings.Contains(err.Error(), `"`+strings.Repeat("x", 64)+`"`) {
		t.Errorf("DecodeObject() error = %q, want the name cut to exactly 64 characters", err)
	}
}

func TestUnmarshalField(t *testing.T) {
	fields, err := DecodeObject([]byte(`{"a":5,"b":"x"}`), "a", "b", "c")
	if err != nil {
		t.Fatalf("DecodeObject() error = %v", err)
	}

	t.Run("field that is present", func(t *testing.T) {
		var got int
		if err := UnmarshalField(fields, "a", &got); err != nil || got != 5 {
			t.Errorf("UnmarshalField(a) = %d, %v, want 5, nil", got, err)
		}
	})
	t.Run("field that is absent keeps the target", func(t *testing.T) {
		got := 7
		if err := UnmarshalField(fields, "c", &got); err != nil || got != 7 {
			t.Errorf("UnmarshalField(c) = %d, %v, want 7, nil", got, err)
		}
	})
	t.Run("field of the wrong type names the field", func(t *testing.T) {
		var got int
		err := UnmarshalField(fields, "b", &got)
		if err == nil || !strings.HasPrefix(err.Error(), "b: ") {
			t.Errorf("UnmarshalField(b) error = %v, want it to start with %q", err, "b: ")
		}
	})
	t.Run("null leaves the target as it was", func(t *testing.T) {
		nulls, err := DecodeObject([]byte(`{"a":null}`), "a")
		if err != nil {
			t.Fatalf("DecodeObject() error = %v", err)
		}
		got := 9
		if err := UnmarshalField(nulls, "a", &got); err != nil || got != 9 {
			t.Errorf("UnmarshalField(null) = %d, %v, want 9, nil", got, err)
		}
	})
	t.Run("error text is capped", func(t *testing.T) {
		huge, err := DecodeObject([]byte(`{"a":`+strings.Repeat("9", 60000)+`}`), "a")
		if err != nil {
			t.Fatalf("DecodeObject() error = %v", err)
		}
		var got string
		err = UnmarshalField(huge, "a", &got)
		if err == nil {
			t.Fatal("UnmarshalField() = nil error, want a type error")
		}
		if len(err.Error()) > len("a: ")+100 {
			t.Errorf("UnmarshalField() error length = %d, want an error capped at %d bytes", len(err.Error()), len("a: ")+100)
		}
	})
}

func TestIsNull(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{"null", `null`, true},
		{"null with whitespace", " \n null\t", true},
		{"empty", ``, false},
		{"string that says null", `"null"`, false},
		{"truncated null", `nul`, false},
		{"zero", `0`, false},
		{"false", `false`, false},
		{"empty object", `{}`, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsNull([]byte(test.raw)); got != test.want {
				t.Errorf("IsNull(%q) = %v, want %v", test.raw, got, test.want)
			}
		})
	}
}
