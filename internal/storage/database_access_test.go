package storage

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"
)

// testDatabaseFileSuffixes lists the three files that SQLite keeps for one
// database, written out here and not taken from the code under test, so a file
// that the code stops checking has a case that fails. The empty suffix is the
// main file.
var testDatabaseFileSuffixes = []string{"", "-wal", "-shm"}

// writeTestFile makes a file with the exact mode, set with chmod so the umask
// does not change it.
func writeTestFile(t *testing.T, path string, mode os.FileMode) {
	t.Helper()

	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %q: %v", path, err)
	}
}

func TestOpenRefusesDatabaseFileThatOthersCanAccess(t *testing.T) {
	modes := []os.FileMode{0o644, 0o640, 0o604, 0o660, 0o666, 0o601, 0o610, 0o620}

	for _, suffix := range testDatabaseFileSuffixes {
		for _, mode := range modes {
			t.Run(fmt.Sprintf("ship8t.db%s %04o", suffix, mode), func(t *testing.T) {
				directory := privateTestDirectory(t)
				path := filepath.Join(directory, "ship8t.db")
				writeTestFile(t, path+suffix, mode)

				database, err := Open(t.Context(), path)
				if err == nil {
					_ = database.Close()
					t.Fatalf("Open() error = nil, want an error for mode %04o", mode)
				}
				if database != nil {
					t.Errorf("Open() database = %v, want nil with an error", database)
				}
				wantCommand := "chmod -- 600 '" + path + suffix + "'"
				// Match the phrase and not the digits, because the temporary
				// directory name already holds the mode text.
				for _, want := range []string{fmt.Sprintf("mode %04o,", mode), wantCommand} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("Open() error = %q, want it to contain %q", err, want)
					}
				}
				if strings.Contains(err.Error(), "\n") {
					t.Errorf("Open() error = %q, want one line", err)
				}

				// The refusal changes nothing, so it neither fixes the mode nor
				// creates a sibling file.
				info, err := os.Lstat(path + suffix)
				if err != nil {
					t.Fatalf("Lstat() error = %v, want nil", err)
				}
				if got := info.Mode().Perm(); got != mode {
					t.Errorf("mode after the refusal = %#o, want %#o", got, mode)
				}
				if got := entryNames(t, directory); strings.Join(got, ",") != "ship8t.db"+suffix {
					t.Errorf("directory entries = %v, want only ship8t.db%s", got, suffix)
				}
			})
		}
	}
}

func TestOpenNamesTheLooseFileAmongOwnerOnlyFiles(t *testing.T) {
	directory := privateTestDirectory(t)
	path := filepath.Join(directory, "ship8t.db")
	writeTestFile(t, path, 0o600)
	writeTestFile(t, path+"-wal", 0o644)
	writeTestFile(t, path+"-shm", 0o600)

	database, err := Open(t.Context(), path)
	if err == nil {
		_ = database.Close()
		t.Fatal("Open() error = nil, want an error")
	}
	if want := "chmod -- 600 '" + path + "-wal'"; !strings.Contains(err.Error(), want) {
		t.Errorf("Open() error = %q, want it to contain %q", err, want)
	}
}

func TestOpenAcceptsExistingOwnerOnlyFiles(t *testing.T) {
	directory := privateTestDirectory(t)
	path := filepath.Join(directory, "ship8t.db")
	for _, suffix := range testDatabaseFileSuffixes {
		writeTestFile(t, path+suffix, 0o600)
	}

	database, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open() error = %v, want nil", err)
	}
	if err := database.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

func TestOpenRefusesDatabaseEntryThatIsNotARegularFile(t *testing.T) {
	kinds := []struct {
		name string
		make func(t *testing.T, directory, path string)
	}{
		{
			name: "symlink to a file",
			make: func(t *testing.T, directory, path string) {
				writeTestFile(t, filepath.Join(directory, "elsewhere"), 0o600)
				if err := os.Symlink("elsewhere", path); err != nil {
					t.Fatalf("symlink: %v", err)
				}
			},
		},
		{
			name: "dangling symlink",
			make: func(t *testing.T, directory, path string) {
				if err := os.Symlink("missing", path); err != nil {
					t.Fatalf("symlink: %v", err)
				}
			},
		},
		{
			name: "directory",
			make: func(t *testing.T, directory, path string) {
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
			},
		},
		{
			name: "named pipe",
			make: func(t *testing.T, directory, path string) {
				if err := syscall.Mkfifo(path, 0o600); err != nil {
					t.Fatalf("mkfifo: %v", err)
				}
			},
		},
	}

	for _, suffix := range testDatabaseFileSuffixes {
		for _, kind := range kinds {
			t.Run(fmt.Sprintf("ship8t.db%s %s", suffix, kind.name), func(t *testing.T) {
				directory := privateTestDirectory(t)
				path := filepath.Join(directory, "ship8t.db")
				kind.make(t, directory, path+suffix)
				before := entryNames(t, directory)

				database, err := Open(t.Context(), path)
				if err == nil {
					_ = database.Close()
					t.Fatal("Open() error = nil, want an error")
				}
				if database != nil {
					t.Errorf("Open() database = %v, want nil with an error", database)
				}
				if want := "is not a regular file"; !strings.Contains(err.Error(), want) {
					t.Errorf("Open() error = %q, want it to contain %q", err, want)
				}
				if want := filepath.Join(directory, "ship8t.db"+suffix); !strings.Contains(err.Error(), want) {
					t.Errorf("Open() error = %q, want it to name %q", err, want)
				}
				if strings.Contains(err.Error(), "\n") {
					t.Errorf("Open() error = %q, want one line", err)
				}

				// A dangling symlink must not make SQLite create its target.
				after := entryNames(t, directory)
				sort.Strings(before)
				if strings.Join(after, ",") != strings.Join(before, ",") {
					t.Errorf("directory entries = %v, want %v", after, before)
				}
			})
		}
	}
}

func TestOpenOmitsCommandForUnprintableDatabasePath(t *testing.T) {
	// Name the file with a newline, so a command in the message could forge a
	// second line.
	path := filepath.Join(privateTestDirectory(t), "bad\nname.db")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Skipf("file system rejects the name: %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	database, err := Open(t.Context(), path)
	if err == nil {
		_ = database.Close()
		t.Fatal("Open() error = nil, want an error")
	}
	message := err.Error()
	if strings.Contains(message, "\n") {
		t.Errorf("Open() error = %q, want one line", message)
	}
	if strings.Contains(message, "chmod --") {
		t.Errorf("Open() error = %q, want no command for an unprintable path", message)
	}
	for _, want := range []string{fmt.Sprintf("%q", path), "mode 0644,", "run chmod 600 on that file"} {
		if !strings.Contains(message, want) {
			t.Errorf("Open() error = %q, want it to contain %q", message, want)
		}
	}
}

func TestOpenCommandFixesTheDatabaseFile(t *testing.T) {
	// A space and a quote in the name prove that the printed command works in
	// a real shell and not only as text.
	path := filepath.Join(privateTestDirectory(t), "my data's.db")
	writeTestFile(t, path, 0o644)

	database, err := Open(t.Context(), path)
	if err == nil {
		_ = database.Close()
		t.Fatal("Open() error = nil, want an error")
	}
	_, command, found := strings.Cut(err.Error(), "run: ")
	if !found {
		t.Fatalf("Open() error = %q, want it to hold a command after %q", err, "run: ")
	}
	if output, err := exec.Command("sh", "-c", command).CombinedOutput(); err != nil {
		t.Fatalf("sh -c %q failed: %v\n%s", command, err, output)
	}

	database, err = Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open() after the command error = %v, want nil", err)
	}
	if err := database.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}
