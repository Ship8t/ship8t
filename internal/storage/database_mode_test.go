package storage

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestOpenCreatesDatabaseFilesWithOwnerOnlyMode(t *testing.T) {
	tests := []struct {
		name  string
		umask int
	}{
		{name: "umask that clears nothing", umask: 0o000},
		{name: "common umask", umask: 0o022},
		// A umask that clears the owner write bit leaves a created file at 0400,
		// so only an explicit chmod gives the file the mode of the P0 rule.
		{name: "umask that clears the owner write bit", umask: 0o300},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ship8t.db")

			previous := syscall.Umask(test.umask)
			database, err := Open(t.Context(), path)
			syscall.Umask(previous)
			if err != nil {
				t.Fatalf("Open() error = %v, want nil", err)
			}
			defer func() {
				if err := database.Close(); err != nil {
					t.Errorf("Close() error = %v, want nil", err)
				}
			}()

			// Write a row so that SQLite creates the -wal and -shm files, which
			// take the mode of the main file.
			if _, err := database.connection.ExecContext(t.Context(), "CREATE TABLE note (text TEXT)"); err != nil {
				t.Fatalf("create table: %v", err)
			}

			for _, suffix := range []string{"", "-wal", "-shm"} {
				info, err := os.Lstat(path + suffix)
				if err != nil {
					t.Fatalf("Lstat(ship8t.db%s) error = %v, want nil", suffix, err)
				}
				if got := info.Mode().Perm(); got != 0o600 {
					t.Errorf("ship8t.db%s permissions = %#o, want %#o", suffix, got, 0o600)
				}
			}
		})
	}
}
