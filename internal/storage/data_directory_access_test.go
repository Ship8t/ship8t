package storage

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// lockEntryAbsent fails the test when the data directory holds a lock entry,
// because a refused directory must stay as it was.
func lockEntryAbsent(t *testing.T, dataDirectory string) {
	t.Helper()

	path := filepath.Join(dataDirectory, "lock")
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Lstat(%q) error = %v, want it to report that the file does not exist", path, err)
	}
}

func TestLockDataDirectoryAcceptsOwnerOnlyMode(t *testing.T) {
	dataDirectory := t.TempDir()
	if err := os.Chmod(dataDirectory, 0o700); err != nil {
		t.Fatalf("chmod data directory: %v", err)
	}

	lock := lockTestDataDirectory(t, dataDirectory)
	if err := lock.Release(); err != nil {
		t.Errorf("Release() error = %v, want nil", err)
	}
}

func TestLockDataDirectoryRefusesDirectoryThatOthersCanAccess(t *testing.T) {
	modes := []os.FileMode{0o755, 0o750, 0o705, 0o770, 0o707, 0o777, 0o701, 0o710, 0o702, 0o720, 0o704, 0o740}

	for _, mode := range modes {
		t.Run(fmt.Sprintf("%04o", mode), func(t *testing.T) {
			dataDirectory := t.TempDir()
			if err := os.Chmod(dataDirectory, mode); err != nil {
				t.Fatalf("chmod data directory: %v", err)
			}

			lock, err := LockDataDirectory(dataDirectory)
			if err == nil {
				_ = lock.Release()
				t.Fatalf("LockDataDirectory() error = nil, want an error for mode %04o", mode)
			}
			if lock != nil {
				t.Errorf("LockDataDirectory() lock = %v, want nil with an error", lock)
			}
			wantCommand := "chmod -- 700 '" + dataDirectory + "'"
			// Match the phrase and not the digits, because the temporary directory
			// name already holds the mode text.
			for _, want := range []string{fmt.Sprintf("mode %04o,", mode), wantCommand} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("LockDataDirectory() error = %q, want it to contain %q", err, want)
				}
			}
			if strings.Contains(err.Error(), "\n") {
				t.Errorf("LockDataDirectory() error = %q, want one line", err)
			}
			lockEntryAbsent(t, dataDirectory)
		})
	}
}

func TestLockDataDirectoryChecksModeOfSymlinkTarget(t *testing.T) {
	parent := t.TempDir()

	t.Run("loose target is refused", func(t *testing.T) {
		target := filepath.Join(parent, "loose")
		if err := os.Mkdir(target, 0o700); err != nil {
			t.Fatalf("make target: %v", err)
		}
		if err := os.Chmod(target, 0o755); err != nil {
			t.Fatalf("chmod target: %v", err)
		}
		link := filepath.Join(parent, "loose-link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatalf("make symlink: %v", err)
		}

		lock, err := LockDataDirectory(link)
		if err == nil {
			_ = lock.Release()
			t.Fatal("LockDataDirectory() error = nil, want an error for a loose target")
		}
		// The command names the path as the caller gave it, and chmod follows it.
		for _, want := range []string{"mode 0755,", "chmod -- 700 '" + link + "'"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("LockDataDirectory() error = %q, want it to contain %q", err, want)
			}
		}
		lockEntryAbsent(t, target)
	})

	t.Run("private target is accepted", func(t *testing.T) {
		target := filepath.Join(parent, "private")
		if err := os.Mkdir(target, 0o700); err != nil {
			t.Fatalf("make target: %v", err)
		}
		link := filepath.Join(parent, "private-link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatalf("make symlink: %v", err)
		}

		lock := lockTestDataDirectory(t, link)
		if err := lock.Release(); err != nil {
			t.Errorf("Release() error = %v, want nil", err)
		}
	})
}

func TestLockDataDirectoryOmitsCommandForUnprintablePath(t *testing.T) {
	// Name the directory with a newline, so a command in the message could
	// forge a second line.
	dataDirectory := filepath.Join(t.TempDir(), "bad\nname")
	if err := os.Mkdir(dataDirectory, 0o755); err != nil {
		t.Skipf("file system rejects the name: %v", err)
	}

	lock, err := LockDataDirectory(dataDirectory)
	if err == nil {
		_ = lock.Release()
		t.Fatal("LockDataDirectory() error = nil, want an error")
	}
	message := err.Error()
	if strings.Contains(message, "\n") {
		t.Errorf("LockDataDirectory() error = %q, want one line", message)
	}
	if strings.Contains(message, "chmod --") {
		t.Errorf("LockDataDirectory() error = %q, want no command for an unprintable path", message)
	}
	for _, want := range []string{fmt.Sprintf("%q", dataDirectory), "mode 0755,", "run chmod 700 on that directory"} {
		if !strings.Contains(message, want) {
			t.Errorf("LockDataDirectory() error = %q, want it to contain %q", message, want)
		}
	}
	lockEntryAbsent(t, dataDirectory)
}

func TestLockDataDirectoryCommandFixesTheDirectory(t *testing.T) {
	// A space and a quote in the name prove that the printed command works in
	// a real shell and not only as text.
	dataDirectory := filepath.Join(t.TempDir(), "my data's")
	if err := os.Mkdir(dataDirectory, 0o700); err != nil {
		t.Fatalf("make data directory: %v", err)
	}
	if err := os.Chmod(dataDirectory, 0o755); err != nil {
		t.Fatalf("chmod data directory: %v", err)
	}

	lock, err := LockDataDirectory(dataDirectory)
	if err == nil {
		_ = lock.Release()
		t.Fatal("LockDataDirectory() error = nil, want an error")
	}
	_, command, found := strings.Cut(err.Error(), "run: ")
	if !found {
		t.Fatalf("LockDataDirectory() error = %q, want it to hold a command after %q", err, "run: ")
	}
	if output, err := exec.Command("sh", "-c", command).CombinedOutput(); err != nil {
		t.Fatalf("sh -c %q failed: %v\n%s", command, err, output)
	}

	lock = lockTestDataDirectory(t, dataDirectory)
	if err := lock.Release(); err != nil {
		t.Errorf("Release() error = %v, want nil", err)
	}
}

func TestLockDataDirectoryReportsUnusableDirectoryBeforeAccess(t *testing.T) {
	// Every path below sits in a loose directory or is loose itself, and the
	// error must still name the first problem.
	tests := []struct {
		name          string
		dataDirectory func(t *testing.T) string
		wantMessage   string
	}{
		{
			name: "missing directory in a loose parent",
			dataDirectory: func(t *testing.T) string {
				parent := t.TempDir()
				if err := os.Chmod(parent, 0o777); err != nil {
					t.Fatalf("chmod parent: %v", err)
				}
				return filepath.Join(parent, "missing")
			},
			wantMessage: "does not exist, so create it",
		},
		{
			name: "loose regular file",
			dataDirectory: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "file")
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatalf("write file: %v", err)
				}
				if err := os.Chmod(path, 0o666); err != nil {
					t.Fatalf("chmod file: %v", err)
				}
				return path
			},
			wantMessage: "is not a directory, so point it at a directory",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lock, err := LockDataDirectory(test.dataDirectory(t))
			if err == nil {
				_ = lock.Release()
				t.Fatal("LockDataDirectory() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), test.wantMessage) {
				t.Errorf("LockDataDirectory() error = %q, want it to contain %q", err, test.wantMessage)
			}
			if strings.Contains(err.Error(), "group or other") {
				t.Errorf("LockDataDirectory() error = %q, want no access message before the earlier problem", err)
			}
		})
	}
}
