package storage

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// execTest runs a statement on the database of a test and fails the test when
// it errors. The statements are constants of the tests.
func execTest(t *testing.T, database *Database, statement string) {
	t.Helper()

	if _, err := database.connection.ExecContext(t.Context(), statement); err != nil {
		t.Fatalf("exec %q: %v", statement, err)
	}
}

// countTest returns the first column of a count query of the database.
func countTest(t *testing.T, database *Database, query string) int {
	t.Helper()

	var count int
	if err := database.connection.QueryRowContext(t.Context(), query).Scan(&count); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return count
}

func TestProjectsTableRejectsInvalidSlugs(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		wantFail bool
	}{
		{name: "NUL byte", id: "a\x00b", wantFail: true},
		{name: "NUL byte at the end", id: "a\x00", wantFail: true},
		{name: "leading hyphen", id: "-site", wantFail: true},
		{name: "trailing hyphen", id: "site-", wantFail: true},
		{name: "64 characters", id: strings.Repeat("a", 64), wantFail: true},
		{name: "uppercase", id: "Site", wantFail: true},
		{name: "accented letter", id: "café", wantFail: true},
		{name: "underscore", id: "my_site", wantFail: true},
		{name: "empty", id: "", wantFail: true},
		{name: "63 characters", id: strings.Repeat("a", 63)},
		{name: "hyphen inside", id: "my-site"},
		{name: "one character", id: "a"},
	}

	database := openTestDatabase(t)
	if err := database.EnsureSchema(t.Context()); err != nil {
		t.Fatalf("EnsureSchema() error = %v, want nil", err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := database.connection.ExecContext(t.Context(), "INSERT INTO projects (id) VALUES (?)", test.id)
			if test.wantFail && err == nil {
				t.Errorf("insert project %q error = nil, want a constraint error", test.id)
			}
			if !test.wantFail && err != nil {
				t.Errorf("insert project %q error = %v, want nil", test.id, err)
			}
		})
	}
}

func TestEnsureSchemaRefusesUnknownDatabases(t *testing.T) {
	tests := []struct {
		name  string
		setup []string
		want  []string
	}{
		{
			name:  "tables and no version",
			setup: []string{"CREATE TABLE note (text TEXT)"},
			want:  []string{"has tables", "version 0", "want version 1"},
		},
		{
			name:  "table whose name only resembles the internal prefix",
			setup: []string{"CREATE TABLE sqlitexnotes (text TEXT)"},
			want:  []string{"has tables", "version 0", "want version 1"},
		},
		{
			name:  "version 2 with tables",
			setup: []string{"CREATE TABLE note (text TEXT)", "PRAGMA user_version = 2"},
			want:  []string{"version 2", "want version 1"},
		},
		{
			name:  "version 5 and no tables",
			setup: []string{"PRAGMA user_version = 5"},
			want:  []string{"version 5", "want version 1"},
		},
		{
			name:  "negative version and no tables",
			setup: []string{"PRAGMA user_version = -1"},
			want:  []string{"version -1", "want version 1"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			database := openTestDatabase(t)
			for _, statement := range test.setup {
				execTest(t, database, statement)
			}
			versionBefore := readPragma(t, database, "user_version")

			err := database.EnsureSchema(t.Context())
			if err == nil {
				t.Fatal("EnsureSchema() error = nil, want a refusal")
			}
			for _, phrase := range test.want {
				if !strings.Contains(err.Error(), phrase) {
					t.Errorf("EnsureSchema() error = %q, want it to contain %q", err, phrase)
				}
			}

			// Expect a refused database to stay as it was.
			if got := readPragma(t, database, "user_version"); got != versionBefore {
				t.Errorf("user_version after refusal = %q, want %q", got, versionBefore)
			}
			if got := countTest(t, database, "SELECT count(*) FROM sqlite_master WHERE name = 'projects'"); got != 0 {
				t.Errorf("projects tables after refusal = %d, want 0", got)
			}
		})
	}
}

func TestEnsureSchemaCreatesVersionOneOnEmptyDatabase(t *testing.T) {
	database := openTestDatabase(t)

	if err := database.EnsureSchema(t.Context()); err != nil {
		t.Fatalf("EnsureSchema() error = %v, want nil", err)
	}

	if got := readPragma(t, database, "user_version"); got != "1" {
		t.Errorf("user_version = %q, want %q", got, "1")
	}
	if got := countTest(t, database, "SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'projects'"); got != 1 {
		t.Errorf("projects tables = %d, want 1", got)
	}
	if got := countTest(t, database, "SELECT strict FROM pragma_table_list WHERE name = 'projects'"); got != 1 {
		t.Errorf("projects strict = %d, want 1", got)
	}
}

func TestEnsureSchemaIgnoresInternalTables(t *testing.T) {
	database := openTestDatabase(t)

	// Dropping a table with AUTOINCREMENT leaves sqlite_sequence behind, which is
	// no table of the operator.
	execTest(t, database, "CREATE TABLE counter (id INTEGER PRIMARY KEY AUTOINCREMENT)")
	execTest(t, database, "DROP TABLE counter")
	if got := countTest(t, database, "SELECT count(*) FROM sqlite_master WHERE name = 'sqlite_sequence'"); got != 1 {
		t.Fatalf("sqlite_sequence entries before the test = %d, want 1", got)
	}

	if err := database.EnsureSchema(t.Context()); err != nil {
		t.Fatalf("EnsureSchema() error = %v, want nil", err)
	}
	if got := readPragma(t, database, "user_version"); got != "1" {
		t.Errorf("user_version = %q, want %q", got, "1")
	}
}

func TestEnsureSchemaKeepsAVersionOneDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ship8t.db")

	first, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("first Open() error = %v, want nil", err)
	}
	if err := first.EnsureSchema(t.Context()); err != nil {
		t.Fatalf("first EnsureSchema() error = %v, want nil", err)
	}
	execTest(t, first, "INSERT INTO projects (id) VALUES ('my-site')")
	if err := first.Close(); err != nil {
		t.Fatalf("first Close() error = %v, want nil", err)
	}

	second, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("second Open() error = %v, want nil", err)
	}
	defer func() {
		if err := second.Close(); err != nil {
			t.Errorf("second Close() error = %v, want nil", err)
		}
	}()

	// Run it twice, because a repeat must change nothing.
	for range 2 {
		if err := second.EnsureSchema(t.Context()); err != nil {
			t.Fatalf("EnsureSchema() on a version 1 database error = %v, want nil", err)
		}
	}
	if got := countTest(t, second, "SELECT count(*) FROM projects"); got != 1 {
		t.Errorf("projects after repeat = %d, want 1", got)
	}
}

func TestEnsureSchemaReturnsContextError(t *testing.T) {
	database := openTestDatabase(t)
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()

	err := database.EnsureSchema(cancelled)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("EnsureSchema() error = %v, want it to wrap context.Canceled", err)
	}
	if got := readPragma(t, database, "user_version"); got != "0" {
		t.Errorf("user_version after a cancelled call = %q, want %q", got, "0")
	}
}

func TestApplySchemaLeavesNothingWhenAStatementFails(t *testing.T) {
	database := openTestDatabase(t)

	statements := []string{
		"CREATE TABLE first (id TEXT NOT NULL PRIMARY KEY) STRICT",
		"CREATE TABLE broken (",
	}
	if err := applySchema(t.Context(), database.connection, statements); err == nil {
		t.Fatal("applySchema() error = nil, want the error of the broken statement")
	}

	if got := readPragma(t, database, "user_version"); got != "0" {
		t.Errorf("user_version after a failed creation = %q, want %q", got, "0")
	}
	if got := countTest(t, database, "SELECT count(*) FROM sqlite_master"); got != 0 {
		t.Errorf("schema objects after a failed creation = %d, want 0", got)
	}

	// Expect a retry on the same database to start from an empty one.
	if err := database.EnsureSchema(t.Context()); err != nil {
		t.Errorf("EnsureSchema() after a failed creation error = %v, want nil", err)
	}
}
