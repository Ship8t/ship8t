package storage

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
