//go:build unix

package scan

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestScanRejectsNonRegularFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "index.html")
	if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0o644); err != nil {
		t.Fatalf("create named pipe: %v", err)
	}

	files, err := Scan(root)
	if err == nil {
		t.Fatalf("Scan() = %v, nil error, want an error", files)
	}
	if !strings.Contains(err.Error(), "pipe") || !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("Scan() error = %q, want it to name pipe and say it is not a regular file", err)
	}
}
