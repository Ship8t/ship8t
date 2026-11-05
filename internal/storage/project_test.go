package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	// The characters just outside each accepted range, which a wrong bound in
	// the rule would let through.
	{name: "slash before zero", id: "a/b"},
	{name: "colon after nine", id: "a:b"},
	{name: "at sign before a", id: "a@b"},
	{name: "backtick before a", id: "a`b"},
	{name: "opening bracket after z", id: "a[b"},
	{name: "opening brace after z", id: "a{b"},
	{name: "comma before hyphen", id: "a,b"},
	{name: "dot after hyphen", id: "a.b"},
	{name: "64 characters", id: strings.Repeat("a", 64)},
	{name: "63 characters", id: strings.Repeat("a", 63), wantValid: true},
	{name: "one character", id: "a", wantValid: true},
	{name: "only digits", id: "123", wantValid: true},
	// The first and last character of each accepted range.
	{name: "zero", id: "0", wantValid: true},
	{name: "nine", id: "9", wantValid: true},
	{name: "z", id: "z", wantValid: true},
	{name: "every letter and digit", id: "abcdefghijklmnopqrstuvwxyz0123456789", wantValid: true},
	{name: "range ends inside a name", id: "z0-9a", wantValid: true},
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

// projectTestSetup returns a database with its schema and a data directory with
// its layout, the state that the server has before it creates a project.
func projectTestSetup(t *testing.T) (*Database, string) {
	t.Helper()

	database := openTestDatabase(t)
	if err := database.EnsureSchema(t.Context()); err != nil {
		t.Fatalf("EnsureSchema() error = %v, want nil", err)
	}
	return database, layoutTestDirectory(t)
}

// projectRows returns the IDs in the projects table, sorted by the query.
func projectRows(t *testing.T, database *Database) []string {
	t.Helper()

	rows, err := database.connection.QueryContext(t.Context(), "SELECT id FROM projects ORDER BY id")
	if err != nil {
		t.Fatalf("query projects: %v", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan project: %v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read projects: %v", err)
	}
	return ids
}

func TestCreateProjectRejectsInvalidID(t *testing.T) {
	ids := []string{"", "..", "../site", "a/b", "a\x00b", "Site", "-site", strings.Repeat("a", 64)}

	for _, id := range ids {
		t.Run(fmt.Sprintf("%q", id), func(t *testing.T) {
			database, dataDirectory := projectTestSetup(t)
			before := treeNames(t, dataDirectory)

			err := database.CreateProject(t.Context(), dataDirectory, id)
			if err == nil {
				t.Fatal("CreateProject() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), "project ID") {
				t.Errorf("CreateProject() error = %q, want the slug message and not a database error", err)
			}
			if got := projectRows(t, database); len(got) != 0 {
				t.Errorf("projects = %v, want none after a refusal", got)
			}
			if got := treeNames(t, dataDirectory); strings.Join(got, ",") != strings.Join(before, ",") {
				t.Errorf("data directory entries = %v, want %v", got, before)
			}
		})
	}
}

func TestCreateProjectRefusesWhatTheDirectoriesRefuse(t *testing.T) {
	tests := []struct {
		name          string
		dataDirectory func(t *testing.T) string
		wantMessage   string
	}{
		{
			name:          "empty data directory path",
			dataDirectory: func(t *testing.T) string { return "" },
			wantMessage:   "path is empty",
		},
		{
			name: "missing layout",
			dataDirectory: func(t *testing.T) string {
				return privateTestDirectory(t)
			},
			wantMessage: "so create the data directory layout first",
		},
		{
			name: "project directory that is a file",
			dataDirectory: func(t *testing.T) string {
				directory := layoutTestDirectory(t)
				if err := os.WriteFile(filepath.Join(directory, "projects", "my-site"), nil, 0o600); err != nil {
					t.Fatalf("write file: %v", err)
				}
				return directory
			},
			wantMessage: "is not a directory, so remove it and start again",
		},
		{
			name: "releases directory that is a symlink",
			dataDirectory: func(t *testing.T) string {
				directory := layoutTestDirectory(t)
				createTestDirectory(t, filepath.Join(directory, "projects", "my-site"))
				if err := os.Symlink(t.TempDir(), filepath.Join(directory, "projects", "my-site", "releases")); err != nil {
					t.Fatalf("make symlink: %v", err)
				}
				return directory
			},
			wantMessage: "is not a directory, so remove it and start again",
		},
		{
			name: "staging directory that others can access",
			dataDirectory: func(t *testing.T) string {
				directory := layoutTestDirectory(t)
				project := filepath.Join(directory, "projects", "my-site")
				createTestDirectory(t, project)
				if err := os.Mkdir(filepath.Join(project, "staging"), 0o755); err != nil {
					t.Fatalf("make directory: %v", err)
				}
				if err := os.Chmod(filepath.Join(project, "staging"), 0o755); err != nil {
					t.Fatalf("chmod directory: %v", err)
				}
				return directory
			},
			wantMessage: "mode 0755,",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			database := openTestDatabase(t)
			if err := database.EnsureSchema(t.Context()); err != nil {
				t.Fatalf("EnsureSchema() error = %v, want nil", err)
			}
			dataDirectory := test.dataDirectory(t)

			err := database.CreateProject(t.Context(), dataDirectory, "my-site")
			if err == nil {
				t.Fatal("CreateProject() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), test.wantMessage) {
				t.Errorf("CreateProject() error = %q, want it to contain %q", err, test.wantMessage)
			}
			// No row may exist for a project whose directories were refused.
			if got := projectRows(t, database); len(got) != 0 {
				t.Errorf("projects = %v, want none after a refusal", got)
			}
		})
	}
}

func TestCreateProjectCreatesTheRowAndTheDirectories(t *testing.T) {
	database, dataDirectory := projectTestSetup(t)

	if err := database.CreateProject(t.Context(), dataDirectory, "my-site"); err != nil {
		t.Fatalf("CreateProject() error = %v, want nil", err)
	}

	if got := projectRows(t, database); strings.Join(got, ",") != "my-site" {
		t.Errorf("projects = %v, want [my-site]", got)
	}
	for _, name := range projectDirectoryNames("my-site") {
		info, err := os.Lstat(filepath.Join(dataDirectory, name))
		if err != nil {
			t.Fatalf("Lstat(%s) error = %v, want nil", name, err)
		}
		if got := info.Mode().Perm(); !info.IsDir() || got != 0o700 {
			t.Errorf("%s mode = %v, want a directory with permissions %#o", name, info.Mode(), 0o700)
		}
	}
}

func TestCreateProjectIsRepeatable(t *testing.T) {
	database, dataDirectory := projectTestSetup(t)
	if err := database.CreateProject(t.Context(), dataDirectory, "my-site"); err != nil {
		t.Fatalf("CreateProject() error = %v, want nil", err)
	}
	content := filepath.Join(dataDirectory, "projects", "my-site", "releases", "keep.txt")
	if err := os.WriteFile(content, []byte("keep"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if err := database.CreateProject(t.Context(), dataDirectory, "my-site"); err != nil {
		t.Fatalf("CreateProject() repeat error = %v, want nil", err)
	}

	if got := projectRows(t, database); strings.Join(got, ",") != "my-site" {
		t.Errorf("projects = %v, want [my-site]", got)
	}
	if got, err := os.ReadFile(content); err != nil || string(got) != "keep" {
		t.Errorf("ReadFile(%q) = %q, %v, want %q", content, got, err, "keep")
	}
}

func TestCreateProjectMakesAnotherProjectBesideIt(t *testing.T) {
	database, dataDirectory := projectTestSetup(t)

	for _, id := range []string{"alpha", "beta"} {
		if err := database.CreateProject(t.Context(), dataDirectory, id); err != nil {
			t.Fatalf("CreateProject(%s) error = %v, want nil", id, err)
		}
	}

	if got := projectRows(t, database); strings.Join(got, ",") != "alpha,beta" {
		t.Errorf("projects = %v, want [alpha beta]", got)
	}
	if got := entryNames(t, filepath.Join(dataDirectory, "projects")); strings.Join(got, ",") != "alpha,beta" {
		t.Errorf("projects entries = %v, want [alpha beta]", got)
	}
}

func TestCreateProjectRepairsMissingDirectoriesOfAnExistingProject(t *testing.T) {
	database, dataDirectory := projectTestSetup(t)
	if err := database.CreateProject(t.Context(), dataDirectory, "my-site"); err != nil {
		t.Fatalf("CreateProject() error = %v, want nil", err)
	}
	if err := os.RemoveAll(filepath.Join(dataDirectory, "projects", "my-site")); err != nil {
		t.Fatalf("remove project directories: %v", err)
	}

	if err := database.CreateProject(t.Context(), dataDirectory, "my-site"); err != nil {
		t.Fatalf("CreateProject() error = %v, want nil", err)
	}

	for _, name := range projectDirectoryNames("my-site") {
		if info, err := os.Lstat(filepath.Join(dataDirectory, name)); err != nil || !info.IsDir() {
			t.Errorf("Lstat(%s) = %v, %v, want a directory", name, info, err)
		}
	}
	if got := projectRows(t, database); strings.Join(got, ",") != "my-site" {
		t.Errorf("projects = %v, want [my-site]", got)
	}
}

func TestCreateProjectAdoptsDirectoriesThatHaveNoRow(t *testing.T) {
	// A crash before the commit, or a failed insert, leaves this state.
	database, dataDirectory := projectTestSetup(t)
	if err := createProjectDirectories(dataDirectory, "my-site"); err != nil {
		t.Fatalf("createProjectDirectories() error = %v, want nil", err)
	}

	if err := database.CreateProject(t.Context(), dataDirectory, "my-site"); err != nil {
		t.Fatalf("CreateProject() error = %v, want nil", err)
	}

	if got := projectRows(t, database); strings.Join(got, ",") != "my-site" {
		t.Errorf("projects = %v, want [my-site]", got)
	}
}

func TestCreateProjectLeavesNoRowWhenTheDirectoriesFail(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can make a directory in a read-only directory")
	}
	database, dataDirectory := projectTestSetup(t)
	projects := filepath.Join(dataDirectory, "projects")
	// A mode of 0500 passes the access check and makes the mkdir fail.
	if err := os.Chmod(projects, 0o500); err != nil {
		t.Fatalf("chmod projects: %v", err)
	}
	// Restore the mode, so the cleanup can remove the directory.
	t.Cleanup(func() { _ = os.Chmod(projects, 0o700) })

	err := database.CreateProject(t.Context(), dataDirectory, "my-site")
	if err == nil {
		t.Fatal("CreateProject() error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("CreateProject() error = %q, want it to contain %q", err, "permission denied")
	}
	if got := projectRows(t, database); len(got) != 0 {
		t.Errorf("projects = %v, want none", got)
	}

	// The database has one connection, so a transaction that stayed open would
	// make this call wait for ever. It must work once the cause is gone.
	if err := os.Chmod(projects, 0o700); err != nil {
		t.Fatalf("chmod projects: %v", err)
	}
	if err := database.CreateProject(t.Context(), dataDirectory, "my-site"); err != nil {
		t.Fatalf("CreateProject() after the failure error = %v, want nil", err)
	}
}

func TestCreateProjectReportsAFailedInsertAndKeepsTheDirectories(t *testing.T) {
	// A database without its schema has no projects table, so the insert fails
	// after the directories exist. The next call must adopt them.
	database := openTestDatabase(t)
	dataDirectory := layoutTestDirectory(t)

	err := database.CreateProject(t.Context(), dataDirectory, "my-site")
	if err == nil {
		t.Fatal("CreateProject() error = nil, want an error")
	}
	for _, want := range []string{"create project: insert:", "no such table"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("CreateProject() error = %q, want it to contain %q", err, want)
		}
	}
	if info, statErr := os.Lstat(filepath.Join(dataDirectory, "projects", "my-site", "releases")); statErr != nil || !info.IsDir() {
		t.Errorf("Lstat(releases) = %v, %v, want the directories to remain", info, statErr)
	}

	if err := database.EnsureSchema(t.Context()); err != nil {
		t.Fatalf("EnsureSchema() error = %v, want nil", err)
	}
	if err := database.CreateProject(t.Context(), dataDirectory, "my-site"); err != nil {
		t.Fatalf("CreateProject() after the failure error = %v, want nil", err)
	}
	if got := projectRows(t, database); strings.Join(got, ",") != "my-site" {
		t.Errorf("projects = %v, want [my-site]", got)
	}
}

func TestCreateProjectReturnsContextError(t *testing.T) {
	database, dataDirectory := projectTestSetup(t)
	before := treeNames(t, dataDirectory)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := database.CreateProject(ctx, dataDirectory, "my-site")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateProject() error = %v, want it to wrap context.Canceled", err)
	}
	if !strings.Contains(err.Error(), "create project: begin transaction:") {
		t.Errorf("CreateProject() error = %q, want it to contain %q", err, "create project: begin transaction:")
	}
	if got := treeNames(t, dataDirectory); strings.Join(got, ",") != strings.Join(before, ",") {
		t.Errorf("data directory entries = %v, want %v", got, before)
	}
	if got := projectRows(t, database); len(got) != 0 {
		t.Errorf("projects = %v, want none", got)
	}
}

func TestCreateProjectIsSafeToCallConcurrently(t *testing.T) {
	database, dataDirectory := projectTestSetup(t)

	const callers = 8
	errs := make([]error, callers)
	var wait sync.WaitGroup
	for index := range callers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errs[index] = database.CreateProject(t.Context(), dataDirectory, "my-site")
		}()
	}
	wait.Wait()

	for index, err := range errs {
		if err != nil {
			t.Errorf("caller %d: CreateProject() error = %v, want nil", index, err)
		}
	}
	if got := projectRows(t, database); strings.Join(got, ",") != "my-site" {
		t.Errorf("projects = %v, want [my-site]", got)
	}
}
