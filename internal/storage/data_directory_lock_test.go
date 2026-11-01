package storage

import (
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
