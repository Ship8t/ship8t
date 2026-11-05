package storage

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"
)

// projectDirectoryNames lists the entries of a project in the order that
// createProjectDirectories makes them, as paths below the data directory.
func projectDirectoryNames(id string) []string {
	return []string{"projects/" + id, "projects/" + id + "/staging", "projects/" + id + "/releases"}
}

// layoutTestDirectory returns a private data directory with its layout made.
func layoutTestDirectory(t *testing.T) string {
	t.Helper()

	dataDirectory := privateTestDirectory(t)
	createTestLayout(t, dataDirectory)
	return dataDirectory
}

// treeNames returns every path below a directory, sorted and relative to it. It
// does not follow a symlink, so it shows what a call made or left alone.
func treeNames(t *testing.T, directory string) []string {
	t.Helper()

	var names []string
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != directory {
			relative, relErr := filepath.Rel(directory, path)
			if relErr != nil {
				return relErr
			}
			names = append(names, relative)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %q: %v", directory, err)
	}
	sort.Strings(names)
	return names
}

func TestCreateProjectDirectoriesRejectsInvalidID(t *testing.T) {
	ids := []string{"", "..", ".", "../site", "a/b", "/site", `a\b`, "a\x00b", "a\nb", "Site", "-site", "site-", strings.Repeat("a", 64)}

	for _, id := range ids {
		t.Run(fmt.Sprintf("%q", id), func(t *testing.T) {
			dataDirectory := layoutTestDirectory(t)
			before := treeNames(t, dataDirectory)

			err := createProjectDirectories(dataDirectory, id)
			if err == nil {
				t.Fatal("createProjectDirectories() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), "project ID") {
				t.Errorf("createProjectDirectories() error = %q, want it to name the project ID", err)
			}
			// A refused ID must not reach the file system, not even to make a
			// directory outside projects/.
			after := treeNames(t, dataDirectory)
			if strings.Join(before, ",") != strings.Join(after, ",") {
				t.Errorf("data directory entries = %v, want %v after a refusal", after, before)
			}
		})
	}
}

func TestCreateProjectDirectoriesCreatesPrivateDirectories(t *testing.T) {
	dataDirectory := layoutTestDirectory(t)

	if err := createProjectDirectories(dataDirectory, "my-site"); err != nil {
		t.Fatalf("createProjectDirectories() error = %v, want nil", err)
	}

	for _, name := range projectDirectoryNames("my-site") {
		info, err := os.Lstat(filepath.Join(dataDirectory, name))
		if err != nil {
			t.Fatalf("Lstat(%s) error = %v, want nil", name, err)
		}
		if !info.IsDir() {
			t.Errorf("%s mode = %v, want a directory", name, info.Mode())
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Errorf("%s permissions = %#o, want %#o", name, got, 0o700)
		}
	}
	want := []string{"projects", "projects/my-site", "projects/my-site/releases", "projects/my-site/staging", "pruning", "quarantine"}
	if got := treeNames(t, dataDirectory); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("data directory entries = %v, want %v", got, want)
	}
}

func TestCreateProjectDirectoriesSetsModeDespiteUmask(t *testing.T) {
	dataDirectory := layoutTestDirectory(t)

	// A umask that clears the owner write bit leaves a new directory with a mode
	// of 0400, so a child cannot be made in it until chmod has run.
	previous := syscall.Umask(0o300)
	err := createProjectDirectories(dataDirectory, "my-site")
	syscall.Umask(previous)
	if err != nil {
		t.Fatalf("createProjectDirectories() error = %v, want nil", err)
	}

	for _, name := range projectDirectoryNames("my-site") {
		info, err := os.Lstat(filepath.Join(dataDirectory, name))
		if err != nil {
			t.Fatalf("Lstat(%s) error = %v, want nil", name, err)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Errorf("%s permissions = %#o, want %#o", name, got, 0o700)
		}
	}
}

func TestCreateProjectDirectoriesIsRepeatable(t *testing.T) {
	dataDirectory := layoutTestDirectory(t)
	if err := createProjectDirectories(dataDirectory, "my-site"); err != nil {
		t.Fatalf("createProjectDirectories() error = %v, want nil", err)
	}
	// Content of an earlier deployment must survive a repeat.
	content := filepath.Join(dataDirectory, "projects", "my-site", "releases", "keep.txt")
	if err := os.WriteFile(content, []byte("keep"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	before := treeNames(t, dataDirectory)

	if err := createProjectDirectories(dataDirectory, "my-site"); err != nil {
		t.Fatalf("createProjectDirectories() repeat error = %v, want nil", err)
	}

	if got := treeNames(t, dataDirectory); strings.Join(got, ",") != strings.Join(before, ",") {
		t.Errorf("data directory entries = %v, want %v after a repeat", got, before)
	}
	if got, err := os.ReadFile(content); err != nil || string(got) != "keep" {
		t.Errorf("ReadFile(%q) = %q, %v, want %q", content, got, err, "keep")
	}
}

func TestCreateProjectDirectoriesCreatesOnlyTheMissingDirectories(t *testing.T) {
	dataDirectory := layoutTestDirectory(t)
	project := filepath.Join(dataDirectory, "projects", "my-site")
	for _, directory := range []string{project, filepath.Join(project, "staging")} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatalf("make directory: %v", err)
		}
		if err := os.Chmod(directory, 0o700); err != nil {
			t.Fatalf("chmod directory: %v", err)
		}
	}
	content := filepath.Join(project, "staging", "keep.txt")
	if err := os.WriteFile(content, []byte("keep"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if err := createProjectDirectories(dataDirectory, "my-site"); err != nil {
		t.Fatalf("createProjectDirectories() error = %v, want nil", err)
	}

	if info, err := os.Lstat(filepath.Join(project, "releases")); err != nil || !info.IsDir() {
		t.Errorf("Lstat(releases) = %v, %v, want a directory", info, err)
	}
	if got, err := os.ReadFile(content); err != nil || string(got) != "keep" {
		t.Errorf("ReadFile(%q) = %q, %v, want %q", content, got, err, "keep")
	}
}

func TestCreateProjectDirectoriesLeavesOtherProjectsAlone(t *testing.T) {
	dataDirectory := layoutTestDirectory(t)
	if err := createProjectDirectories(dataDirectory, "alpha"); err != nil {
		t.Fatalf("createProjectDirectories(alpha) error = %v, want nil", err)
	}
	content := filepath.Join(dataDirectory, "projects", "alpha", "releases", "keep.txt")
	if err := os.WriteFile(content, []byte("keep"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if err := createProjectDirectories(dataDirectory, "beta"); err != nil {
		t.Fatalf("createProjectDirectories(beta) error = %v, want nil", err)
	}

	if got, err := os.ReadFile(content); err != nil || string(got) != "keep" {
		t.Errorf("ReadFile(%q) = %q, %v, want %q", content, got, err, "keep")
	}
	want := []string{"alpha", "beta"}
	if got := entryNames(t, filepath.Join(dataDirectory, "projects")); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("projects entries = %v, want %v", got, want)
	}
}

func TestCreateProjectDirectoriesRejectsUnusableDataDirectory(t *testing.T) {
	tests := []struct {
		name          string
		dataDirectory func(t *testing.T) string
		wantMessage   string
	}{
		{
			name:          "empty path",
			dataDirectory: func(t *testing.T) string { return "" },
			wantMessage:   "path is empty",
		},
		{
			name: "directory that does not exist",
			dataDirectory: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing")
			},
			wantMessage: "does not exist, so create it",
		},
		{
			name: "path that names a regular file",
			dataDirectory: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "file")
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatalf("write file: %v", err)
				}
				return path
			},
			wantMessage: "is not a directory, so point it at a directory",
		},
		{
			name: "directory that others can access",
			dataDirectory: func(t *testing.T) string {
				directory := layoutTestDirectory(t)
				if err := os.Chmod(directory, 0o755); err != nil {
					t.Fatalf("chmod directory: %v", err)
				}
				return directory
			},
			wantMessage: "mode 0755,",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := createProjectDirectories(test.dataDirectory(t), "my-site")
			if err == nil {
				t.Fatal("createProjectDirectories() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), test.wantMessage) {
				t.Errorf("createProjectDirectories() error = %q, want it to contain %q", err, test.wantMessage)
			}
		})
	}
}

func TestCreateProjectDirectoriesRequiresTheLayout(t *testing.T) {
	dataDirectory := privateTestDirectory(t)
	before := treeNames(t, dataDirectory)

	err := createProjectDirectories(dataDirectory, "my-site")
	if err == nil {
		t.Fatal("createProjectDirectories() error = nil, want an error")
	}
	for _, want := range []string{fmt.Sprintf("%q", filepath.Join(dataDirectory, "projects")), "does not exist, so create the data directory layout first"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("createProjectDirectories() error = %q, want it to contain %q", err, want)
		}
	}
	// The project must not make projects/ for the layout.
	if got := treeNames(t, dataDirectory); strings.Join(got, ",") != strings.Join(before, ",") {
		t.Errorf("data directory entries = %v, want %v", got, before)
	}
}

// badEntryKinds lists what can sit where a directory belongs. The loose case is
// a real directory that group or other can use.
var badEntryKinds = []struct {
	name        string
	make        func(t *testing.T, dataDirectory, path string)
	wantMessage string
}{
	{
		name: "regular file",
		make: func(t *testing.T, dataDirectory, path string) {
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatalf("write file: %v", err)
			}
		},
		wantMessage: "is not a directory, so remove it and start again",
	},
	{
		name: "symlink to a directory in the data directory",
		make: func(t *testing.T, dataDirectory, path string) {
			relative, err := filepath.Rel(filepath.Dir(path), filepath.Join(dataDirectory, "quarantine"))
			if err != nil {
				t.Fatalf("relative path: %v", err)
			}
			if err := os.Symlink(relative, path); err != nil {
				t.Fatalf("make symlink: %v", err)
			}
		},
		wantMessage: "is not a directory, so remove it and start again",
	},
	{
		name: "symlink to a directory outside the data directory",
		make: func(t *testing.T, dataDirectory, path string) {
			if err := os.Symlink(t.TempDir(), path); err != nil {
				t.Fatalf("make symlink: %v", err)
			}
		},
		wantMessage: "is not a directory, so remove it and start again",
	},
	{
		name: "symlink to a file that does not exist",
		make: func(t *testing.T, dataDirectory, path string) {
			if err := os.Symlink("nowhere", path); err != nil {
				t.Fatalf("make symlink: %v", err)
			}
		},
		wantMessage: "is not a directory, so remove it and start again",
	},
	{
		name: "named pipe",
		make: func(t *testing.T, dataDirectory, path string) {
			if err := syscall.Mkfifo(path, 0o600); err != nil {
				t.Fatalf("make named pipe: %v", err)
			}
		},
		wantMessage: "is not a directory, so remove it and start again",
	},
	{
		name: "directory that others can access",
		make: func(t *testing.T, dataDirectory, path string) {
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatalf("make directory: %v", err)
			}
			if err := os.Chmod(path, 0o755); err != nil {
				t.Fatalf("chmod directory: %v", err)
			}
		},
		wantMessage: "mode 0755,",
	},
}

func TestCreateProjectDirectoriesRejectsBadEntries(t *testing.T) {
	// The entry that is bad, and the entries that must exist before it.
	entries := []struct {
		name    string
		path    string
		present []string
	}{
		{name: "projects", path: "projects"},
		{name: "project directory", path: "projects/my-site"},
		{name: "staging", path: "projects/my-site/staging", present: []string{"projects/my-site"}},
		{name: "releases", path: "projects/my-site/releases", present: []string{"projects/my-site"}},
	}

	for _, entry := range entries {
		for _, kind := range badEntryKinds {
			t.Run(kind.name+" as "+entry.name, func(t *testing.T) {
				dataDirectory := privateTestDirectory(t)
				for _, name := range []string{"quarantine", "pruning", "projects"} {
					if name == "projects" && entry.path == "projects" {
						continue
					}
					createTestDirectory(t, filepath.Join(dataDirectory, name))
				}
				for _, name := range entry.present {
					createTestDirectory(t, filepath.Join(dataDirectory, name))
				}
				path := filepath.Join(dataDirectory, entry.path)
				kind.make(t, dataDirectory, path)
				before := treeNames(t, dataDirectory)

				err := createProjectDirectories(dataDirectory, "my-site")
				if err == nil {
					t.Fatal("createProjectDirectories() error = nil, want an error")
				}
				if !strings.Contains(err.Error(), kind.wantMessage) {
					t.Errorf("createProjectDirectories() error = %q, want it to contain %q", err, kind.wantMessage)
				}
				if !strings.Contains(err.Error(), "mode 0755,") && !strings.Contains(err.Error(), fmt.Sprintf("%q", path)) {
					t.Errorf("createProjectDirectories() error = %q, want it to name %q", err, path)
				}
				// Every entry is checked before the first mkdir, so a refusal
				// creates nothing, and a symlink is never followed.
				if got := treeNames(t, dataDirectory); strings.Join(got, ",") != strings.Join(before, ",") {
					t.Errorf("data directory entries = %v, want %v after a refusal", got, before)
				}
			})
		}
	}
}

func TestCreateProjectDirectoriesReportsAMkdirFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can make a directory in a read-only directory")
	}
	dataDirectory := layoutTestDirectory(t)
	projects := filepath.Join(dataDirectory, "projects")
	// A mode of 0500 passes the access check, because no bit of group or other
	// is set, and it makes the mkdir fail.
	if err := os.Chmod(projects, 0o500); err != nil {
		t.Fatalf("chmod projects: %v", err)
	}
	// Restore the mode, so the test cleanup can remove the directory.
	t.Cleanup(func() { _ = os.Chmod(projects, 0o700) })

	err := createProjectDirectories(dataDirectory, "my-site")
	if err == nil {
		t.Fatal("createProjectDirectories() error = nil, want an error")
	}
	for _, want := range []string{"create project directories:", "permission denied"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("createProjectDirectories() error = %q, want it to contain %q", err, want)
		}
	}
	if got := entryNames(t, projects); len(got) != 0 {
		t.Errorf("projects entries = %v, want none", got)
	}
}

func TestCreateProjectDirectoriesRequiresPruning(t *testing.T) {
	// A project is moved into pruning/ when it is pruned, so the directory must
	// exist before a project does.
	dataDirectory := privateTestDirectory(t)
	createTestDirectory(t, filepath.Join(dataDirectory, "projects"))
	createTestDirectory(t, filepath.Join(dataDirectory, "quarantine"))
	before := treeNames(t, dataDirectory)

	err := createProjectDirectories(dataDirectory, "my-site")
	if err == nil {
		t.Fatal("createProjectDirectories() error = nil, want an error")
	}
	for _, want := range []string{fmt.Sprintf("%q", filepath.Join(dataDirectory, "pruning")), "does not exist, so create the data directory layout first"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("createProjectDirectories() error = %q, want it to contain %q", err, want)
		}
	}
	if got := treeNames(t, dataDirectory); strings.Join(got, ",") != strings.Join(before, ",") {
		t.Errorf("data directory entries = %v, want %v after a refusal", got, before)
	}
}

func TestCreateProjectDirectoriesRejectsBadPruning(t *testing.T) {
	for _, kind := range badEntryKinds {
		t.Run(kind.name, func(t *testing.T) {
			dataDirectory := privateTestDirectory(t)
			createTestDirectory(t, filepath.Join(dataDirectory, "projects"))
			createTestDirectory(t, filepath.Join(dataDirectory, "quarantine"))
			path := filepath.Join(dataDirectory, "pruning")
			kind.make(t, dataDirectory, path)
			before := treeNames(t, dataDirectory)

			err := createProjectDirectories(dataDirectory, "my-site")
			if err == nil {
				t.Fatal("createProjectDirectories() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), kind.wantMessage) {
				t.Errorf("createProjectDirectories() error = %q, want it to contain %q", err, kind.wantMessage)
			}
			if got := treeNames(t, dataDirectory); strings.Join(got, ",") != strings.Join(before, ",") {
				t.Errorf("data directory entries = %v, want %v after a refusal", got, before)
			}
		})
	}
}

func TestCreateProjectDirectoriesDoesNotCreateThroughSymlink(t *testing.T) {
	dataDirectory := layoutTestDirectory(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dataDirectory, "projects", "my-site")); err != nil {
		t.Fatalf("make symlink: %v", err)
	}

	if err := createProjectDirectories(dataDirectory, "my-site"); err == nil {
		t.Fatal("createProjectDirectories() error = nil, want an error")
	}

	if got := entryNames(t, outside); len(got) != 0 {
		t.Errorf("entries outside the data directory = %v, want none", got)
	}
}

func TestCreateProjectDirectoriesKeepsTheErrorOnOneLine(t *testing.T) {
	// The newline sits in the data directory name, so every path holds it.
	dataDirectory := filepath.Join(t.TempDir(), "bad\nname")
	if err := os.Mkdir(dataDirectory, 0o700); err != nil {
		t.Skipf("file system rejects the name: %v", err)
	}
	createTestLayout(t, dataDirectory)
	path := filepath.Join(dataDirectory, "projects", "my-site")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("make directory: %v", err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("chmod directory: %v", err)
	}

	err := createProjectDirectories(dataDirectory, "my-site")
	if err == nil {
		t.Fatal("createProjectDirectories() error = nil, want an error")
	}
	message := err.Error()
	if strings.Contains(message, "\n") {
		t.Errorf("createProjectDirectories() error = %q, want one line", message)
	}
	if strings.Contains(message, "chmod --") {
		t.Errorf("createProjectDirectories() error = %q, want no command for an unprintable path", message)
	}
	for _, want := range []string{fmt.Sprintf("%q", path), "mode 0755,"} {
		if !strings.Contains(message, want) {
			t.Errorf("createProjectDirectories() error = %q, want it to contain %q", message, want)
		}
	}
}

// createTestDirectory makes a directory with mode 0700 and stops the test when
// it cannot. It sets the mode with chmod, so the umask does not change it.
func createTestDirectory(t *testing.T, path string) {
	t.Helper()

	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("make directory %q: %v", path, err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatalf("chmod directory %q: %v", path, err)
	}
}
