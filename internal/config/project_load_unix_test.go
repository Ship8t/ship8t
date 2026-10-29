//go:build unix

package config

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestLoadProjectFileDoesNotBlockOnANamedPipe(t *testing.T) {
	directory := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(directory, "ship8t.json"), 0o644); err != nil {
		t.Fatalf("create named pipe: %v", err)
	}

	// Opening a pipe with no writer blocks for ever, so a check that comes after
	// the open would hang the deploy.
	done := make(chan error, 1)
	go func() {
		_, err := LoadProjectFile(directory)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Errorf("LoadProjectFile() error = %v, want it to say the file is not a regular file", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("LoadProjectFile() blocked on a named pipe")
	}
}

func TestLoadProjectFileFailsOnAnUnreadableFile(t *testing.T) {
	directory := t.TempDir()
	writeProjectFile(t, directory, `{"project":"demo"}`)
	locked := filepath.Join(directory, "ship8t.json")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatalf("remove access to the file: %v", err)
	}
	// A process with the right to read any file, such as root, cannot fail here.
	if _, err := os.ReadFile(locked); err == nil {
		t.Skip("the file is still readable, run the test as a normal user")
	}

	_, err := LoadProjectFile(directory)
	if err == nil {
		t.Fatal("LoadProjectFile() = nil error, want an error")
	}
	for _, want := range []string{"ship8t.json", "permission denied"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("LoadProjectFile() error = %q, want it to contain %q", err, want)
		}
	}
}

func TestLoadProjectFileQuotesDirectoriesWithNewlines(t *testing.T) {
	// A directory name with a newline would forge a second line in an error that
	// prints it without quoting.
	parent := t.TempDir()
	directory := filepath.Join(parent, "bad\nproject \"x\": ok")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Skipf("file names with a newline are not available: %v", err)
	}
	writeProjectFile(t, directory, `{"project":"A"}`)

	_, err := LoadProjectFile(directory)
	if err == nil {
		t.Fatal("LoadProjectFile() = nil error, want an error")
	}
	if lines := strings.Split(err.Error(), "\n"); len(lines) != 1 {
		t.Errorf("LoadProjectFile() error = %q, want a single line, so a directory name cannot forge extra lines", err)
	}
	if !strings.Contains(err.Error(), `\n`) {
		t.Errorf("LoadProjectFile() error = %q, want the quoted directory name with its escaped newline", err)
	}
}
