package storage

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"
)

// createTestLayout creates the layout and stops the test when it fails.
func createTestLayout(t *testing.T, dataDirectory string) {
	t.Helper()

	if err := CreateDataDirectoryLayout(dataDirectory); err != nil {
		t.Fatalf("CreateDataDirectoryLayout(%q) error = %v, want nil", dataDirectory, err)
	}
}

// entryNames returns the sorted names inside a directory.
func entryNames(t *testing.T, directory string) []string {
	t.Helper()

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read directory %q: %v", directory, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

func TestCreateDataDirectoryLayoutCreatesPrivateDirectories(t *testing.T) {
	dataDirectory := privateTestDirectory(t)

	createTestLayout(t, dataDirectory)

	for _, name := range layoutDirectoryNames {
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
}

func TestCreateDataDirectoryLayoutSetsModeDespiteUmask(t *testing.T) {
	dataDirectory := privateTestDirectory(t)

	// A umask that clears the owner write bit makes Mkdir leave a mode of 0400.
	// Only an explicit chmod gives the directory the mode of ADR 0003.
	previous := syscall.Umask(0o300)
	err := CreateDataDirectoryLayout(dataDirectory)
	syscall.Umask(previous)
	if err != nil {
		t.Fatalf("CreateDataDirectoryLayout() error = %v, want nil", err)
	}

	for _, name := range layoutDirectoryNames {
		info, err := os.Lstat(filepath.Join(dataDirectory, name))
		if err != nil {
			t.Fatalf("Lstat(%s) error = %v, want nil", name, err)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Errorf("%s permissions = %#o, want %#o", name, got, 0o700)
		}
	}
}

func TestCreateDataDirectoryLayoutCreatesNothingElse(t *testing.T) {
	dataDirectory := privateTestDirectory(t)

	createTestLayout(t, dataDirectory)

	want := []string{"projects", "pruning", "quarantine"}
	if got := entryNames(t, dataDirectory); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("data directory entries = %v, want %v", got, want)
	}
	for _, name := range layoutDirectoryNames {
		if got := entryNames(t, filepath.Join(dataDirectory, name)); len(got) != 0 {
			t.Errorf("%s entries = %v, want none", name, got)
		}
	}
}

func TestCreateDataDirectoryLayoutIsRepeatable(t *testing.T) {
	dataDirectory := privateTestDirectory(t)
	createTestLayout(t, dataDirectory)

	// Put content in each directory, because a repeat must leave it alone.
	for _, name := range layoutDirectoryNames {
		if err := os.WriteFile(filepath.Join(dataDirectory, name, "kept"), []byte("kept"), 0o600); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	createTestLayout(t, dataDirectory)

	for _, name := range layoutDirectoryNames {
		got, err := os.ReadFile(filepath.Join(dataDirectory, name, "kept"))
		if err != nil || string(got) != "kept" {
			t.Errorf("%s/kept = %q, %v, want it left as it was", name, got, err)
		}
	}
}

func TestCreateDataDirectoryLayoutCreatesOnlyTheMissingDirectories(t *testing.T) {
	dataDirectory := privateTestDirectory(t)
	if err := os.Mkdir(filepath.Join(dataDirectory, "quarantine"), 0o700); err != nil {
		t.Fatalf("make directory: %v", err)
	}

	createTestLayout(t, dataDirectory)

	want := []string{"projects", "pruning", "quarantine"}
	if got := entryNames(t, dataDirectory); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("data directory entries = %v, want %v", got, want)
	}
}

func TestCreateDataDirectoryLayoutWorksBesideTheLock(t *testing.T) {
	dataDirectory := privateTestDirectory(t)
	lock := lockTestDataDirectory(t, dataDirectory)
	defer func() {
		if err := lock.Release(); err != nil {
			t.Errorf("Release() error = %v, want nil", err)
		}
	}()

	createTestLayout(t, dataDirectory)

	want := []string{"lock", "projects", "pruning", "quarantine"}
	if got := entryNames(t, dataDirectory); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("data directory entries = %v, want %v", got, want)
	}
}

func TestCreateDataDirectoryLayoutRejectsUnusableDataDirectory(t *testing.T) {
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
				directory := t.TempDir()
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
			dataDirectory := test.dataDirectory(t)

			err := CreateDataDirectoryLayout(dataDirectory)
			if err == nil {
				t.Fatal("CreateDataDirectoryLayout() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), test.wantMessage) {
				t.Errorf("CreateDataDirectoryLayout() error = %q, want it to contain %q", err, test.wantMessage)
			}
			// A refused data directory must stay as it was, and a missing one must
			// not be created.
			if dataDirectory == "" {
				return
			}
			if info, statErr := os.Stat(dataDirectory); statErr == nil && info.IsDir() {
				if got := entryNames(t, dataDirectory); len(got) != 0 {
					t.Errorf("data directory entries = %v, want none after a refusal", got)
				}
			} else if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) && !errors.Is(statErr, syscall.ENOTDIR) {
				t.Errorf("Stat(%q) error = %v", dataDirectory, statErr)
			}
		})
	}
}

func TestCreateDataDirectoryLayoutDoesNotCreateMissingDataDirectory(t *testing.T) {
	dataDirectory := filepath.Join(t.TempDir(), "missing")

	if err := CreateDataDirectoryLayout(dataDirectory); err == nil {
		t.Fatal("CreateDataDirectoryLayout() error = nil, want an error")
	}
	if _, err := os.Stat(dataDirectory); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(%q) error = %v, want it to report that the directory does not exist", dataDirectory, err)
	}
}

func TestCreateDataDirectoryLayoutRejectsEntryThatIsNotADirectory(t *testing.T) {
	tests := []struct {
		name string
		make func(t *testing.T, dataDirectory, entry string)
	}{
		{
			name: "regular file",
			make: func(t *testing.T, dataDirectory, entry string) {
				if err := os.WriteFile(filepath.Join(dataDirectory, entry), nil, 0o600); err != nil {
					t.Fatalf("write file: %v", err)
				}
			},
		},
		{
			name: "symlink to a directory in the data directory",
			make: func(t *testing.T, dataDirectory, entry string) {
				if err := os.Mkdir(filepath.Join(dataDirectory, "elsewhere"), 0o700); err != nil {
					t.Fatalf("make directory: %v", err)
				}
				if err := os.Symlink("elsewhere", filepath.Join(dataDirectory, entry)); err != nil {
					t.Fatalf("make symlink: %v", err)
				}
			},
		},
		{
			name: "symlink to a directory outside the data directory",
			make: func(t *testing.T, dataDirectory, entry string) {
				if err := os.Symlink(t.TempDir(), filepath.Join(dataDirectory, entry)); err != nil {
					t.Fatalf("make symlink: %v", err)
				}
			},
		},
		{
			name: "symlink to a file that does not exist",
			make: func(t *testing.T, dataDirectory, entry string) {
				if err := os.Symlink("nowhere", filepath.Join(dataDirectory, entry)); err != nil {
					t.Fatalf("make symlink: %v", err)
				}
			},
		},
		{
			name: "named pipe",
			make: func(t *testing.T, dataDirectory, entry string) {
				if err := syscall.Mkfifo(filepath.Join(dataDirectory, entry), 0o600); err != nil {
					t.Fatalf("make named pipe: %v", err)
				}
			},
		},
	}

	for _, test := range tests {
		for _, entry := range layoutDirectoryNames {
			t.Run(test.name+" as "+entry, func(t *testing.T) {
				dataDirectory := privateTestDirectory(t)
				test.make(t, dataDirectory, entry)

				err := CreateDataDirectoryLayout(dataDirectory)
				if err == nil {
					t.Fatal("CreateDataDirectoryLayout() error = nil, want an error")
				}
				want := fmt.Sprintf("%q is not a directory, so remove it and start again", filepath.Join(dataDirectory, entry))
				if !strings.Contains(err.Error(), want) {
					t.Errorf("CreateDataDirectoryLayout() error = %q, want it to contain %q", err, want)
				}
				// Nothing is created before every existing entry has been checked.
				for _, other := range layoutDirectoryNames {
					if other == entry {
						continue
					}
					path := filepath.Join(dataDirectory, other)
					if _, statErr := os.Lstat(path); !errors.Is(statErr, fs.ErrNotExist) {
						t.Errorf("Lstat(%q) error = %v, want it to report that the entry does not exist", path, statErr)
					}
				}
			})
		}
	}
}

func TestCreateDataDirectoryLayoutDoesNotCreateThroughSymlink(t *testing.T) {
	dataDirectory := privateTestDirectory(t)
	if err := os.Symlink("nowhere", filepath.Join(dataDirectory, "pruning")); err != nil {
		t.Fatalf("make symlink: %v", err)
	}

	if err := CreateDataDirectoryLayout(dataDirectory); err == nil {
		t.Fatal("CreateDataDirectoryLayout() error = nil, want an error")
	}

	target := filepath.Join(dataDirectory, "nowhere")
	if _, err := os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Lstat(%q) error = %v, want it to report that the file does not exist", target, err)
	}
}

func TestCreateDataDirectoryLayoutRefusesEntryThatOthersCanAccess(t *testing.T) {
	modes := []os.FileMode{0o755, 0o750, 0o705, 0o770, 0o707, 0o777, 0o701, 0o710}

	for _, entry := range layoutDirectoryNames {
		for _, mode := range modes {
			t.Run(fmt.Sprintf("%s %04o", entry, mode), func(t *testing.T) {
				dataDirectory := privateTestDirectory(t)
				path := filepath.Join(dataDirectory, entry)
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatalf("make directory: %v", err)
				}
				if err := os.Chmod(path, mode); err != nil {
					t.Fatalf("chmod directory: %v", err)
				}

				err := CreateDataDirectoryLayout(dataDirectory)
				if err == nil {
					t.Fatal("CreateDataDirectoryLayout() error = nil, want an error")
				}
				wantCommand := "chmod -- 700 '" + path + "'"
				for _, want := range []string{fmt.Sprintf("mode %04o,", mode), wantCommand} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("CreateDataDirectoryLayout() error = %q, want it to contain %q", err, want)
					}
				}
				if strings.Contains(err.Error(), "\n") {
					t.Errorf("CreateDataDirectoryLayout() error = %q, want one line", err)
				}

				// The refused directory keeps its mode, and nothing else appears.
				info, statErr := os.Lstat(path)
				if statErr != nil {
					t.Fatalf("Lstat(%s) error = %v, want nil", entry, statErr)
				}
				if got := info.Mode().Perm(); got != mode {
					t.Errorf("%s permissions = %#o, want %#o left as they were", entry, got, mode)
				}
				if got := entryNames(t, dataDirectory); len(got) != 1 || got[0] != entry {
					t.Errorf("data directory entries = %v, want only %s", got, entry)
				}
			})
		}
	}
}

func TestCreateDataDirectoryLayoutOmitsCommandForUnprintableEntryPath(t *testing.T) {
	// The newline sits in the data directory name, so the entry path holds it too.
	dataDirectory := filepath.Join(t.TempDir(), "bad\nname")
	if err := os.Mkdir(dataDirectory, 0o700); err != nil {
		t.Skipf("file system rejects the name: %v", err)
	}
	path := filepath.Join(dataDirectory, "pruning")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("make directory: %v", err)
	}

	err := CreateDataDirectoryLayout(dataDirectory)
	if err == nil {
		t.Fatal("CreateDataDirectoryLayout() error = nil, want an error")
	}
	message := err.Error()
	if strings.Contains(message, "\n") {
		t.Errorf("CreateDataDirectoryLayout() error = %q, want one line", message)
	}
	if strings.Contains(message, "chmod --") {
		t.Errorf("CreateDataDirectoryLayout() error = %q, want no command for an unprintable path", message)
	}
	for _, want := range []string{fmt.Sprintf("%q", path), "mode 0755,", "run chmod 700 on that directory"} {
		if !strings.Contains(message, want) {
			t.Errorf("CreateDataDirectoryLayout() error = %q, want it to contain %q", message, want)
		}
	}
}

func TestCreateDataDirectoryLayoutDoesNotNameTheLockInCheckErrors(t *testing.T) {
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
			name: "directory that cannot be looked up",
			dataDirectory: func(t *testing.T) string {
				if os.Geteuid() == 0 {
					t.Skip("root can look up every path")
				}
				parent := t.TempDir()
				if err := os.Chmod(parent, 0o000); err != nil {
					t.Fatalf("chmod parent: %v", err)
				}
				// Restore the mode, so the test cleanup can remove the directory.
				t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
				return filepath.Join(parent, "data")
			},
			wantMessage: "permission denied",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := CreateDataDirectoryLayout(test.dataDirectory(t))
			if err == nil {
				t.Fatal("CreateDataDirectoryLayout() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), test.wantMessage) {
				t.Errorf("CreateDataDirectoryLayout() error = %q, want it to contain %q", err, test.wantMessage)
			}
			// No lock was tried, so a message about locking would mislead.
			if strings.Contains(err.Error(), "lock data directory") {
				t.Errorf("CreateDataDirectoryLayout() error = %q, want no mention of the lock", err)
			}
		})
	}
}
