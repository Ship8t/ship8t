package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// openTestDatabase opens a database in a new temporary directory and closes it
// when the test ends.
func openTestDatabase(t *testing.T) *Database {
	t.Helper()

	database, err := Open(t.Context(), filepath.Join(t.TempDir(), "ship8t.db"))
	if err != nil {
		t.Fatalf("Open() error = %v, want nil", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("Close() error = %v, want nil", err)
		}
	})
	return database
}

// readPragma returns the current value of a pragma of the database. The name
// is a constant of the test, because SQLite takes no placeholder for it.
func readPragma(t *testing.T, database *Database, name string) string {
	t.Helper()

	var value string
	err := database.connection.QueryRowContext(t.Context(), "PRAGMA "+name).Scan(&value)
	if err != nil {
		t.Fatalf("read pragma %s: %v", name, err)
	}
	return value
}

func TestOpenAppliesSettings(t *testing.T) {
	tests := []struct {
		pragma string
		want   string
	}{
		{pragma: "journal_mode", want: "wal"},
		{pragma: "synchronous", want: "2"},
		{pragma: "foreign_keys", want: "1"},
		{pragma: "busy_timeout", want: "5000"},
	}

	database := openTestDatabase(t)
	for _, test := range tests {
		t.Run(test.pragma, func(t *testing.T) {
			if got := readPragma(t, database, test.pragma); got != test.want {
				t.Errorf("PRAGMA %s = %q, want %q", test.pragma, got, test.want)
			}
		})
	}
}

func TestOpenUsesOneConnection(t *testing.T) {
	database := openTestDatabase(t)

	if got := database.connection.Stats().MaxOpenConnections; got != 1 {
		t.Errorf("MaxOpenConnections = %d, want 1", got)
	}
}

func TestOpenEnforcesForeignKeys(t *testing.T) {
	database := openTestDatabase(t)

	setup := []string{
		"CREATE TABLE parent (id INTEGER PRIMARY KEY)",
		"CREATE TABLE child (parent_id INTEGER NOT NULL REFERENCES parent (id))",
	}
	for _, statement := range setup {
		if _, err := database.connection.ExecContext(t.Context(), statement); err != nil {
			t.Fatalf("setup %q: %v", statement, err)
		}
	}

	_, err := database.connection.ExecContext(t.Context(), "INSERT INTO child (parent_id) VALUES (1)")
	if err == nil {
		t.Error("insert of a child row without a parent succeeded, want a foreign key error")
	}
}

func TestOpenSerializesTransactions(t *testing.T) {
	database := openTestDatabase(t)

	first, err := database.connection.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("begin first transaction: %v", err)
	}
	defer func() {
		if err := first.Rollback(); err != nil {
			t.Errorf("roll back first transaction: %v", err)
		}
	}()

	waitContext, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	// Expect the second transaction to wait, because there is one connection.
	second, err := database.connection.BeginTx(waitContext, nil)
	if err == nil {
		_ = second.Rollback()
		t.Fatal("second transaction began while the first was open, want it to wait")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("second transaction error = %v, want context.DeadlineExceeded", err)
	}
}

func TestOpenKeepsDataAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ship8t.db")

	first, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("first Open() error = %v, want nil", err)
	}
	if _, err := first.connection.ExecContext(t.Context(), "CREATE TABLE note (text TEXT NOT NULL)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := first.connection.ExecContext(t.Context(), "INSERT INTO note (text) VALUES ('kept')"); err != nil {
		t.Fatalf("insert row: %v", err)
	}
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

	var text string
	if err := second.connection.QueryRowContext(t.Context(), "SELECT text FROM note").Scan(&text); err != nil {
		t.Fatalf("read row after reopen: %v", err)
	}
	if text != "kept" {
		t.Errorf("text after reopen = %q, want %q", text, "kept")
	}
	if got := readPragma(t, second, "journal_mode"); got != "wal" {
		t.Errorf("journal_mode after reopen = %q, want %q", got, "wal")
	}
}

func TestOpenAcceptsRelativePath(t *testing.T) {
	t.Chdir(t.TempDir())

	database, err := Open(t.Context(), "ship8t.db")
	if err != nil {
		t.Fatalf("Open() error = %v, want nil", err)
	}
	if err := database.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}

	if _, err := os.Stat("ship8t.db"); err != nil {
		t.Errorf("database file in the working directory: %v", err)
	}
}

func TestOpenKeepsPathCharactersLiteral(t *testing.T) {
	// Check that characters with a meaning in a URI do not change which file
	// SQLite opens, because a path is a file name.
	tests := []struct {
		name     string
		fileName string
	}{
		{name: "space", fileName: "ship8t data.db"},
		{name: "question mark", fileName: "ship8t?mode=ro.db"},
		{name: "hash", fileName: "ship8t#fragment.db"},
		{name: "percent escape", fileName: "ship8t%2Fdata.db"},
		{name: "colon", fileName: "ship8t:data.db"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), test.fileName)

			database, err := Open(t.Context(), path)
			if err != nil {
				t.Fatalf("Open() error = %v, want nil", err)
			}
			if err := database.Close(); err != nil {
				t.Errorf("Close() error = %v, want nil", err)
			}

			if _, err := os.Stat(path); err != nil {
				t.Errorf("database file %q: %v", test.fileName, err)
			}
		})
	}
}

func TestOpenRejectsUnusablePath(t *testing.T) {
	tests := []struct {
		name        string
		path        func(t *testing.T) string
		wantMessage string
	}{
		{
			name:        "empty path",
			path:        func(t *testing.T) string { return "" },
			wantMessage: "path is empty",
		},
		{
			name: "path with a NUL byte",
			path: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "ship8t\x00.db")
			},
			wantMessage: "NUL",
		},
		{
			name: "directory that does not exist",
			path: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing", "ship8t.db")
			},
			wantMessage: "unable to open database file",
		},
		{
			name:        "path that names a directory",
			path:        func(t *testing.T) string { return t.TempDir() },
			wantMessage: "unable to open database file",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			database, err := Open(t.Context(), test.path(t))
			if err == nil {
				_ = database.Close()
				t.Fatal("Open() error = nil, want an error")
			}
			if database != nil {
				t.Errorf("Open() database = %v, want nil with an error", database)
			}
			if !strings.Contains(err.Error(), test.wantMessage) {
				t.Errorf("Open() error = %q, want it to contain %q", err, test.wantMessage)
			}
		})
	}
}

func TestOpenReturnsContextError(t *testing.T) {
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()

	database, err := Open(cancelled, filepath.Join(t.TempDir(), "ship8t.db"))
	if err == nil {
		_ = database.Close()
		t.Fatal("Open() with a cancelled context error = nil, want an error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Open() error = %v, want it to wrap context.Canceled", err)
	}
}
