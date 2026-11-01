package storage

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// fakeFileInfo reports a chosen value from Sys and leaves every other method to
// the real file info that it wraps.
type fakeFileInfo struct {
	fs.FileInfo
	sys any
}

func (info fakeFileInfo) Sys() any { return info.sys }

// fakeDirectoryInfo returns the file info of a real directory with a chosen
// Sys value, so a test can stand for a directory on any device.
func fakeDirectoryInfo(t *testing.T, sys any) fs.FileInfo {
	t.Helper()

	info, err := os.Lstat(t.TempDir())
	if err != nil {
		t.Fatalf("Lstat() error = %v, want nil", err)
	}
	return fakeFileInfo{FileInfo: info, sys: sys}
}

func TestSameFilesystemRejectsDirectoriesOnDifferentDevices(t *testing.T) {
	projects := fakeDirectoryInfo(t, &syscall.Stat_t{Dev: 4242})
	pruning := fakeDirectoryInfo(t, &syscall.Stat_t{Dev: 9191})

	err := sameFilesystem("/data/projects", "/data/pruning", projects, pruning)
	if err == nil {
		t.Fatal("sameFilesystem() error = nil for directories on different devices, want an error")
	}
	message := err.Error()
	for _, want := range []string{`"/data/projects"`, `"/data/pruning"`, "are on different filesystems, so put them on one filesystem"} {
		if !strings.Contains(message, want) {
			t.Errorf("sameFilesystem() error = %q, want it to contain %q", message, want)
		}
	}
	// The message says which directories differ and never the device IDs.
	for _, device := range []string{"4242", "9191"} {
		if strings.Contains(message, device) {
			t.Errorf("sameFilesystem() error = %q, want no device ID %s", message, device)
		}
	}
	if strings.Contains(message, "\n") {
		t.Errorf("sameFilesystem() error = %q, want one line", message)
	}
}

func TestSameFilesystemQuotesUnprintablePaths(t *testing.T) {
	projectsPath := "/data/bad\nname/projects"
	pruningPath := "/data/bad\u202ename/pruning"
	projects := fakeDirectoryInfo(t, &syscall.Stat_t{Dev: 1})
	pruning := fakeDirectoryInfo(t, &syscall.Stat_t{Dev: 2})

	err := sameFilesystem(projectsPath, pruningPath, projects, pruning)
	if err == nil {
		t.Fatal("sameFilesystem() error = nil, want an error")
	}
	message := err.Error()
	if strings.ContainsAny(message, "\n\u202e") {
		t.Errorf("sameFilesystem() error = %q, want one line with no raw control or format character", message)
	}
	for _, want := range []string{fmt.Sprintf("%q", projectsPath), fmt.Sprintf("%q", pruningPath)} {
		if !strings.Contains(message, want) {
			t.Errorf("sameFilesystem() error = %q, want it to contain %q", message, want)
		}
	}
}

func TestSameFilesystemAcceptsDirectoriesOnOneDevice(t *testing.T) {
	projects := fakeDirectoryInfo(t, &syscall.Stat_t{Dev: 7})
	pruning := fakeDirectoryInfo(t, &syscall.Stat_t{Dev: 7})

	if err := sameFilesystem("/data/projects", "/data/pruning", projects, pruning); err != nil {
		t.Fatalf("sameFilesystem() error = %v, want nil", err)
	}
}

func TestSameFilesystemReportsInfoWithoutDevice(t *testing.T) {
	device := &syscall.Stat_t{Dev: 7}
	tests := []struct {
		name    string
		sys     func() (projects, pruning any)
		wantErr string
	}{
		{
			name:    "projects has no stat result",
			sys:     func() (any, any) { return nil, device },
			wantErr: `"/data/projects"`,
		},
		{
			name:    "pruning has no stat result",
			sys:     func() (any, any) { return device, nil },
			wantErr: `"/data/pruning"`,
		},
		{
			name:    "projects has a value of another type",
			sys:     func() (any, any) { return "not a stat result", device },
			wantErr: `"/data/projects"`,
		},
		{
			name:    "pruning has a stat result that is not a pointer",
			sys:     func() (any, any) { return device, syscall.Stat_t{Dev: 7} },
			wantErr: `"/data/pruning"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectsSys, pruningSys := test.sys()
			projects := fakeDirectoryInfo(t, projectsSys)
			pruning := fakeDirectoryInfo(t, pruningSys)

			err := sameFilesystem("/data/projects", "/data/pruning", projects, pruning)
			if err == nil {
				t.Fatal("sameFilesystem() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Errorf("sameFilesystem() error = %q, want it to name %s", err, test.wantErr)
			}
			if strings.Contains(err.Error(), "different filesystems") {
				t.Errorf("sameFilesystem() error = %q, want the unreadable device and not a mismatch", err)
			}
		})
	}
}

func TestCheckLayoutFilesystemAcceptsRealLayout(t *testing.T) {
	dataDirectory := privateTestDirectory(t)
	createTestLayout(t, dataDirectory)

	if err := CheckLayoutFilesystem(dataDirectory); err != nil {
		t.Fatalf("CheckLayoutFilesystem(%q) error = %v, want nil", dataDirectory, err)
	}
	// The check only reads, so a repeat passes and nothing appears.
	if err := CheckLayoutFilesystem(dataDirectory); err != nil {
		t.Fatalf("CheckLayoutFilesystem(%q) repeat error = %v, want nil", dataDirectory, err)
	}
	want := []string{"projects", "pruning", "quarantine"}
	if got := entryNames(t, dataDirectory); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("data directory entries = %v, want %v", got, want)
	}
}

func TestCheckLayoutFilesystemRejectsUnusableDataDirectory(t *testing.T) {
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
				directory := privateTestDirectory(t)
				createTestLayout(t, directory)
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
			err := CheckLayoutFilesystem(test.dataDirectory(t))
			if err == nil {
				t.Fatal("CheckLayoutFilesystem() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), test.wantMessage) {
				t.Errorf("CheckLayoutFilesystem() error = %q, want it to contain %q", err, test.wantMessage)
			}
		})
	}
}

func TestCheckLayoutFilesystemRejectsMissingEntry(t *testing.T) {
	for _, name := range []string{"projects", "pruning"} {
		t.Run("missing "+name, func(t *testing.T) {
			dataDirectory := privateTestDirectory(t)
			createTestLayout(t, dataDirectory)
			path := filepath.Join(dataDirectory, name)
			if err := os.Remove(path); err != nil {
				t.Fatalf("remove %s: %v", name, err)
			}

			err := CheckLayoutFilesystem(dataDirectory)
			if err == nil {
				t.Fatal("CheckLayoutFilesystem() error = nil, want an error")
			}
			for _, want := range []string{fmt.Sprintf("%q", path), "does not exist, so create the data directory layout first"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("CheckLayoutFilesystem() error = %q, want it to contain %q", err, want)
				}
			}
		})
	}
}

func TestCheckLayoutFilesystemRejectsEntryThatIsNotADirectory(t *testing.T) {
	tests := []struct {
		name  string
		entry string
		make  func(t *testing.T, path string)
	}{
		{
			name:  "symlinked projects",
			entry: "projects",
			make: func(t *testing.T, path string) {
				if err := os.Symlink(t.TempDir(), path); err != nil {
					t.Fatalf("symlink: %v", err)
				}
			},
		},
		{
			name:  "symlinked pruning",
			entry: "pruning",
			make: func(t *testing.T, path string) {
				if err := os.Symlink(t.TempDir(), path); err != nil {
					t.Fatalf("symlink: %v", err)
				}
			},
		},
		{
			name:  "projects that is a file",
			entry: "projects",
			make: func(t *testing.T, path string) {
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatalf("write file: %v", err)
				}
			},
		},
		{
			name:  "pruning that is a file",
			entry: "pruning",
			make: func(t *testing.T, path string) {
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatalf("write file: %v", err)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dataDirectory := privateTestDirectory(t)
			createTestLayout(t, dataDirectory)
			path := filepath.Join(dataDirectory, test.entry)
			if err := os.Remove(path); err != nil {
				t.Fatalf("remove %s: %v", test.entry, err)
			}
			test.make(t, path)

			err := CheckLayoutFilesystem(dataDirectory)
			if err == nil {
				t.Fatal("CheckLayoutFilesystem() error = nil, want an error")
			}
			for _, want := range []string{fmt.Sprintf("%q", path), "is not a directory, so remove it and start again"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("CheckLayoutFilesystem() error = %q, want it to contain %q", err, want)
				}
			}
		})
	}
}

func TestCheckLayoutFilesystemRefusesEntryThatOthersCanAccess(t *testing.T) {
	for _, name := range []string{"projects", "pruning"} {
		t.Run(name, func(t *testing.T) {
			dataDirectory := privateTestDirectory(t)
			createTestLayout(t, dataDirectory)
			if err := os.Chmod(filepath.Join(dataDirectory, name), 0o755); err != nil {
				t.Fatalf("chmod %s: %v", name, err)
			}

			err := CheckLayoutFilesystem(dataDirectory)
			if err == nil {
				t.Fatal("CheckLayoutFilesystem() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), "mode 0755,") {
				t.Errorf("CheckLayoutFilesystem() error = %q, want it to contain %q", err, "mode 0755,")
			}
		})
	}
}

func TestCheckLayoutFilesystemQuotesUnprintablePath(t *testing.T) {
	// The newline sits in the data directory name, so the entry paths hold it too.
	dataDirectory := filepath.Join(t.TempDir(), "bad\nname")
	if err := os.Mkdir(dataDirectory, 0o700); err != nil {
		t.Skipf("file system rejects the name: %v", err)
	}

	err := CheckLayoutFilesystem(dataDirectory)
	if err == nil {
		t.Fatal("CheckLayoutFilesystem() error = nil, want an error for the missing layout")
	}
	message := err.Error()
	if strings.Contains(message, "\n") {
		t.Errorf("CheckLayoutFilesystem() error = %q, want one line", message)
	}
	want := fmt.Sprintf("%q", filepath.Join(dataDirectory, "projects"))
	if !strings.Contains(message, want) {
		t.Errorf("CheckLayoutFilesystem() error = %q, want it to contain %q", message, want)
	}
}
