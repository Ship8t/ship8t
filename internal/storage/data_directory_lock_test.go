package storage

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// lockTestDataDirectory locks a data directory and stops the test when it
// cannot. The test releases the lock itself.
func lockTestDataDirectory(t *testing.T, dataDirectory string) *DataDirectoryLock {
	t.Helper()

	lock, err := LockDataDirectory(dataDirectory)
	if err != nil {
		t.Fatalf("LockDataDirectory(%q) error = %v, want nil", dataDirectory, err)
	}
	return lock
}

func TestLockDataDirectoryExcludesSecondLock(t *testing.T) {
	dataDirectory := t.TempDir()
	first := lockTestDataDirectory(t, dataDirectory)

	second, err := LockDataDirectory(dataDirectory)
	if err == nil {
		_ = second.Release()
		_ = first.Release()
		t.Fatal("second LockDataDirectory() error = nil, want an error while the first lock is held")
	}
	if second != nil {
		t.Errorf("second LockDataDirectory() lock = %v, want nil with an error", second)
	}
	if !strings.Contains(err.Error(), "in use by another process") {
		t.Errorf("second LockDataDirectory() error = %q, want it to say the directory is in use by another process", err)
	}

	if err := first.Release(); err != nil {
		t.Errorf("Release() error = %v, want nil", err)
	}
}

func TestLockDataDirectoryAllowsLockAfterRelease(t *testing.T) {
	dataDirectory := t.TempDir()

	first := lockTestDataDirectory(t, dataDirectory)
	if err := first.Release(); err != nil {
		t.Fatalf("Release() error = %v, want nil", err)
	}

	second := lockTestDataDirectory(t, dataDirectory)
	if err := second.Release(); err != nil {
		t.Errorf("second Release() error = %v, want nil", err)
	}
}

func TestLockDataDirectoryRejectsUnusableDirectory(t *testing.T) {
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
			wantMessage: "not a directory",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lock, err := LockDataDirectory(test.dataDirectory(t))
			if err == nil {
				_ = lock.Release()
				t.Fatal("LockDataDirectory() error = nil, want an error")
			}
			if lock != nil {
				t.Errorf("LockDataDirectory() lock = %v, want nil with an error", lock)
			}
			if !strings.Contains(err.Error(), test.wantMessage) {
				t.Errorf("LockDataDirectory() error = %q, want it to contain %q", err, test.wantMessage)
			}
		})
	}
}

func TestLockDataDirectoryRejectsLockEntryThatIsNotARegularFile(t *testing.T) {
	tests := []struct {
		name string
		make func(t *testing.T, dataDirectory string)
	}{
		{
			name: "symlink to a file in the directory",
			make: func(t *testing.T, dataDirectory string) {
				target := filepath.Join(dataDirectory, "target")
				if err := os.WriteFile(target, nil, 0o600); err != nil {
					t.Fatalf("write target: %v", err)
				}
				if err := os.Symlink("target", filepath.Join(dataDirectory, "lock")); err != nil {
					t.Fatalf("make symlink: %v", err)
				}
			},
		},
		{
			name: "symlink to a file that does not exist",
			make: func(t *testing.T, dataDirectory string) {
				if err := os.Symlink("nowhere", filepath.Join(dataDirectory, "lock")); err != nil {
					t.Fatalf("make symlink: %v", err)
				}
			},
		},
		{
			name: "directory",
			make: func(t *testing.T, dataDirectory string) {
				if err := os.Mkdir(filepath.Join(dataDirectory, "lock"), 0o700); err != nil {
					t.Fatalf("make directory: %v", err)
				}
			},
		},
		{
			name: "named pipe",
			make: func(t *testing.T, dataDirectory string) {
				if err := syscall.Mkfifo(filepath.Join(dataDirectory, "lock"), 0o600); err != nil {
					t.Fatalf("make named pipe: %v", err)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dataDirectory := t.TempDir()
			test.make(t, dataDirectory)

			lock, err := LockDataDirectory(dataDirectory)
			if err == nil {
				_ = lock.Release()
				t.Fatal("LockDataDirectory() error = nil, want an error")
			}
			if lock != nil {
				t.Errorf("LockDataDirectory() lock = %v, want nil with an error", lock)
			}
			if !strings.Contains(err.Error(), "is not a regular file") {
				t.Errorf("LockDataDirectory() error = %q, want it to contain %q", err, "is not a regular file")
			}
		})
	}
}

func TestLockDataDirectoryDoesNotCreateFileThroughSymlink(t *testing.T) {
	dataDirectory := t.TempDir()
	if err := os.Symlink("nowhere", filepath.Join(dataDirectory, "lock")); err != nil {
		t.Fatalf("make symlink: %v", err)
	}

	if lock, err := LockDataDirectory(dataDirectory); err == nil {
		_ = lock.Release()
		t.Fatal("LockDataDirectory() error = nil, want an error")
	}

	// A refused lock must not leave a new file behind at the link target.
	target := filepath.Join(dataDirectory, "nowhere")
	if _, err := os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Lstat(%q) error = %v, want it to report that the file does not exist", target, err)
	}
}

func TestLockDataDirectoryCreatesPrivateLockFile(t *testing.T) {
	dataDirectory := t.TempDir()
	lock := lockTestDataDirectory(t, dataDirectory)
	defer func() {
		if err := lock.Release(); err != nil {
			t.Errorf("Release() error = %v, want nil", err)
		}
	}()

	info, err := os.Lstat(filepath.Join(dataDirectory, "lock"))
	if err != nil {
		t.Fatalf("Lstat(lock) error = %v, want nil", err)
	}
	if !info.Mode().IsRegular() {
		t.Errorf("lock file mode = %v, want a regular file", info.Mode())
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("lock file permissions = %#o, want %#o", got, 0o600)
	}
}

func TestLockDataDirectoryDoesNotCreateMissingDirectory(t *testing.T) {
	dataDirectory := filepath.Join(t.TempDir(), "missing")

	if lock, err := LockDataDirectory(dataDirectory); err == nil {
		_ = lock.Release()
		t.Fatal("LockDataDirectory() error = nil, want an error")
	}

	if _, err := os.Stat(dataDirectory); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(%q) error = %v, want it to report that the directory does not exist", dataDirectory, err)
	}
}

func TestLockDataDirectoryKeepsDirectoriesIndependent(t *testing.T) {
	firstDirectory := t.TempDir()
	secondDirectory := t.TempDir()

	first := lockTestDataDirectory(t, firstDirectory)
	second := lockTestDataDirectory(t, secondDirectory)

	if err := first.Release(); err != nil {
		t.Errorf("first Release() error = %v, want nil", err)
	}
	if err := second.Release(); err != nil {
		t.Errorf("second Release() error = %v, want nil", err)
	}
}
