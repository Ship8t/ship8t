//go:build unix

package scan

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// nameWithNewline is a file name that would forge a second line in an error
// that prints it without quoting.
const nameWithNewline = "bad\nscan \"index.html\": ok"

func TestScanRejectsNonRegularFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "index.html")
	if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0o644); err != nil {
		t.Fatalf("create named pipe: %v", err)
	}

	files, err := Scan(root, Options{})
	if err == nil {
		t.Fatalf("Scan() = %v, nil error, want an error", files)
	}
	if !strings.Contains(err.Error(), "pipe") || !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("Scan() error = %q, want it to name pipe and say it is not a regular file", err)
	}
}

func TestScanErrorsQuoteNamesWithNewlines(t *testing.T) {
	tests := []struct {
		name      string
		build     func(t *testing.T, root string)
		wantInErr string
	}{
		{
			name: "symlink",
			build: func(t *testing.T, root string) {
				writeFile(t, root, "index.html")
				symlink(t, root, "index.html", nameWithNewline)
			},
			wantInErr: "symlinks are not allowed",
		},
		{
			name: "non-regular file",
			build: func(t *testing.T, root string) {
				if err := syscall.Mkfifo(filepath.Join(root, nameWithNewline), 0o644); err != nil {
					t.Skipf("file names with a newline are not available: %v", err)
				}
			},
			wantInErr: "not a regular file",
		},
		{
			name: "unreadable directory",
			build: func(t *testing.T, root string) {
				directory := filepath.Join(root, nameWithNewline)
				if err := os.Mkdir(directory, 0o755); err != nil {
					t.Skipf("file names with a newline are not available: %v", err)
				}
				// Restore access, so the test can remove the directory.
				t.Cleanup(func() { _ = os.Chmod(directory, 0o755) })
				if err := os.Chmod(directory, 0o000); err != nil {
					t.Fatalf("remove access to the directory: %v", err)
				}
				// A process with the right to read any file, such as root, cannot fail here.
				if _, err := os.ReadDir(directory); err == nil {
					t.Skip("the directory is still readable, run the test as a normal user")
				}
			},
			wantInErr: "permission denied",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			test.build(t, root)

			files, err := Scan(root, Options{})
			if err == nil {
				t.Fatalf("Scan() = %v, nil error, want an error", files)
			}
			if lines := strings.Split(err.Error(), "\n"); len(lines) != 1 {
				t.Errorf("Scan() error = %q, want a single line, so a file name cannot forge extra lines", err)
			}
			if !strings.Contains(err.Error(), test.wantInErr) {
				t.Errorf("Scan() error = %q, want it to contain %q", err, test.wantInErr)
			}
			if !strings.Contains(err.Error(), `\n`) {
				t.Errorf("Scan() error = %q, want the quoted file name with its escaped newline", err)
			}
		})
	}
}

func TestScanFailsOnUnreadableFileAndQuotesItsName(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "index.html")
	writeFile(t, root, nameWithNewline)
	locked := filepath.Join(root, nameWithNewline)
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatalf("remove access to the file: %v", err)
	}
	// A process with the right to read any file, such as root, cannot fail here.
	if _, err := os.ReadFile(locked); err == nil {
		t.Skip("the file is still readable, run the test as a normal user")
	}

	files, err := Scan(root, Options{})
	if err == nil {
		t.Fatalf("Scan() = %v, nil error, want an error", files)
	}
	if lines := strings.Split(err.Error(), "\n"); len(lines) != 1 {
		t.Errorf("Scan() error = %q, want a single line, so a file name cannot forge extra lines", err)
	}
	for _, want := range []string{"permission denied", `\n`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Scan() error = %q, want it to contain %q", err, want)
		}
	}
}
