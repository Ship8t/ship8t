package storage

import (
	"strings"
	"testing"
)

// projectIDCases lists the IDs that the slug rule of ADR 0003 decides. The
// cases are written out and not taken from the code under test.
var projectIDCases = []struct {
	name      string
	id        string
	wantValid bool
}{
	{name: "empty", id: ""},
	{name: "NUL byte inside", id: "a\x00b"},
	{name: "NUL byte at the end", id: "a\x00"},
	{name: "dot", id: "."},
	{name: "two dots", id: ".."},
	{name: "parent segment", id: "../site"},
	{name: "slash inside", id: "a/b"},
	{name: "leading slash", id: "/site"},
	{name: "backslash", id: `a\b`},
	{name: "newline", id: "a\nb"},
	{name: "space", id: "my site"},
	{name: "leading hyphen", id: "-site"},
	{name: "trailing hyphen", id: "site-"},
	{name: "only a hyphen", id: "-"},
	{name: "uppercase", id: "Site"},
	{name: "accented letter", id: "café"},
	{name: "underscore", id: "my_site"},
	{name: "right to left override", id: "a\u202eb"},
	{name: "64 characters", id: strings.Repeat("a", 64)},
	{name: "63 characters", id: strings.Repeat("a", 63), wantValid: true},
	{name: "one character", id: "a", wantValid: true},
	{name: "only digits", id: "123", wantValid: true},
	{name: "hyphen inside", id: "my-site", wantValid: true},
	{name: "two hyphens inside", id: "my--site", wantValid: true},
}

func TestValidateProjectID(t *testing.T) {
	for _, test := range projectIDCases {
		t.Run(test.name, func(t *testing.T) {
			err := validateProjectID(test.id)
			if test.wantValid && err != nil {
				t.Errorf("validateProjectID(%q) error = %v, want nil", test.id, err)
			}
			if !test.wantValid && err == nil {
				t.Errorf("validateProjectID(%q) error = nil, want an error", test.id)
			}
		})
	}
}

func TestValidateProjectIDNamesTheRule(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		wantMessage string
	}{
		{name: "empty", id: "", wantMessage: "must be 1 to 63 characters, got 0"},
		{name: "too long", id: strings.Repeat("a", 64), wantMessage: "must be 1 to 63 characters, got 64"},
		{name: "bad character", id: "My-site", wantMessage: "must use only lowercase letters, digits, and hyphens"},
		{name: "NUL byte", id: "a\x00b", wantMessage: "must use only lowercase letters, digits, and hyphens"},
		{name: "leading hyphen", id: "-site", wantMessage: "must not start or end with a hyphen"},
		{name: "trailing hyphen", id: "site-", wantMessage: "must not start or end with a hyphen"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateProjectID(test.id)
			if err == nil {
				t.Fatalf("validateProjectID(%q) error = nil, want an error", test.id)
			}
			if !strings.Contains(err.Error(), "project ID") {
				t.Errorf("validateProjectID(%q) error = %q, want it to name the project ID", test.id, err)
			}
			if !strings.Contains(err.Error(), test.wantMessage) {
				t.Errorf("validateProjectID(%q) error = %q, want it to contain %q", test.id, err, test.wantMessage)
			}
		})
	}
}

func TestValidateProjectIDKeepsTheErrorShort(t *testing.T) {
	// The value comes from a request, so an error must never repeat all of it.
	for _, id := range []string{
		strings.Repeat("a", 1<<20),
		"A" + strings.Repeat("a", 5000),
		"a-" + strings.Repeat("\n", 5000),
		strings.Repeat("-", 5000),
	} {
		err := validateProjectID(id)
		if err == nil {
			t.Fatalf("validateProjectID(%.20q...) error = nil, want an error", id)
		}
		if got := len(err.Error()); got > 200 {
			t.Errorf("validateProjectID(%.20q...) error length = %d, want 200 or fewer", id, got)
		}
	}
}

func TestValidateProjectIDKeepsTheErrorOnOneLine(t *testing.T) {
	for _, id := range []string{"a\nb", "a\rb", "a\x00b", "a\u202eb", "\n"} {
		err := validateProjectID(id)
		if err == nil {
			t.Fatalf("validateProjectID(%q) error = nil, want an error", id)
		}
		if strings.ContainsAny(err.Error(), "\n\r\x00\u202e") {
			t.Errorf("validateProjectID(%q) error = %q, want one line with no raw control character", id, err)
		}
	}
}

func TestValidateProjectIDAgreesWithTheProjectsTable(t *testing.T) {
	// The CHECK of the table is the backstop of this rule, so both must give the
	// same verdict. A difference means that one of them drifted.
	database := openTestDatabase(t)
	if err := database.EnsureSchema(t.Context()); err != nil {
		t.Fatalf("EnsureSchema() error = %v, want nil", err)
	}

	for _, test := range projectIDCases {
		t.Run(test.name, func(t *testing.T) {
			_, insertErr := database.connection.ExecContext(t.Context(), "INSERT INTO projects (id) VALUES (?)", test.id)
			if (insertErr == nil) != test.wantValid {
				t.Errorf("insert project %q error = %v, want valid = %v", test.id, insertErr, test.wantValid)
			}
			if (validateProjectID(test.id) == nil) != (insertErr == nil) {
				t.Errorf("validateProjectID(%q) and the table disagree, insert error = %v", test.id, insertErr)
			}
		})
	}
}
