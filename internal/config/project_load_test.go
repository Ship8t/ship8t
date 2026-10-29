package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeProjectFile creates ship8t.json with the given content in directory.
func writeProjectFile(t *testing.T, directory, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, "ship8t.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("write ship8t.json: %v", err)
	}
}

func TestLoadProjectFileReadsTheFile(t *testing.T) {
	directory := t.TempDir()
	writeProjectFile(t, directory, `{"project":"my-site","directory":"dist","ignore":["*.map"],"serve":{"spa_fallback":true}}`)

	got, err := LoadProjectFile(directory)
	if err != nil {
		t.Fatalf("LoadProjectFile() error = %v", err)
	}
	want := ProjectFile{Project: "my-site", Directory: "dist", Ignore: []string{"*.map"}, Serve: Serve{SPAFallback: true}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LoadProjectFile() = %+v, want %+v", got, want)
	}
}

func TestLoadProjectFileTreatsAMissingFileAsNoSettings(t *testing.T) {
	directory := t.TempDir()
	// Other files must not stand in for ship8t.json.
	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}

	got, err := LoadProjectFile(directory)
	if err != nil {
		t.Fatalf("LoadProjectFile() error = %v, want nil for a directory without ship8t.json", err)
	}
	if !reflect.DeepEqual(got, ProjectFile{}) {
		t.Errorf("LoadProjectFile() = %+v, want the zero value", got)
	}
}

func TestLoadProjectFileReadsOnlyTheDirectoryItIsGiven(t *testing.T) {
	parent := t.TempDir()
	writeProjectFile(t, parent, `{"project":"parent-project"}`)
	child := filepath.Join(parent, "site")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatalf("create project directory: %v", err)
	}

	got, err := LoadProjectFile(child)
	if err != nil {
		t.Fatalf("LoadProjectFile() error = %v", err)
	}
	if got.Project != "" {
		t.Errorf("LoadProjectFile() project = %q, want no settings from a parent directory", got.Project)
	}
}

func TestLoadProjectFileFailsOnABadProjectDirectory(t *testing.T) {
	notADirectory := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(notADirectory, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tests := []struct {
		name      string
		directory string
	}{
		{"missing directory", filepath.Join(t.TempDir(), "missing")},
		{"empty path", ""},
		{"file instead of a directory", notADirectory},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := LoadProjectFile(test.directory)
			if err == nil {
				t.Fatalf("LoadProjectFile(%q) = %+v, nil error, want an error", test.directory, got)
			}
			if !strings.Contains(err.Error(), "project directory") {
				t.Errorf("LoadProjectFile(%q) error = %q, want it to name the project directory", test.directory, err)
			}
		})
	}
}

func TestLoadProjectFileRefusesAnythingButARegularFile(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.json"), []byte(`{"project":"stolen"}`), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	tests := []struct {
		name  string
		build func(t *testing.T, directory string)
	}{
		{"directory", func(t *testing.T, directory string) {
			if err := os.Mkdir(filepath.Join(directory, "ship8t.json"), 0o755); err != nil {
				t.Fatalf("create directory: %v", err)
			}
		}},
		{"symlink to a file in the project", func(t *testing.T, directory string) {
			if err := os.WriteFile(filepath.Join(directory, "real.json"), []byte(`{"project":"real"}`), 0o644); err != nil {
				t.Fatalf("write real.json: %v", err)
			}
			symlinkOrSkip(t, "real.json", filepath.Join(directory, "ship8t.json"))
		}},
		{"symlink to a file outside the project", func(t *testing.T, directory string) {
			symlinkOrSkip(t, filepath.Join(outside, "secret.json"), filepath.Join(directory, "ship8t.json"))
		}},
		{"dangling symlink", func(t *testing.T, directory string) {
			symlinkOrSkip(t, "missing.json", filepath.Join(directory, "ship8t.json"))
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			test.build(t, directory)

			got, err := LoadProjectFile(directory)
			if err == nil {
				t.Fatalf("LoadProjectFile() = %+v, nil error, want an error", got)
			}
			if !strings.Contains(err.Error(), "not a regular file") {
				t.Errorf("LoadProjectFile() error = %q, want it to say the file is not a regular file", err)
			}
			if got.Project != "" {
				t.Errorf("LoadProjectFile() project = %q, want the zero value with an error", got.Project)
			}
		})
	}
}

// symlinkOrSkip creates a symlink, and skips the test where symlinks are not
// available.
func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
}

func TestLoadProjectFileReportsInvalidContent(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantInErr []string
	}{
		{"token", `{"token":"s3cret-value"}`, []string{"token", "version control"}},
		{"unknown field", `{"server":"x"}`, []string{"unknown field"}},
		{"directory that climbs out", `{"directory":"../elsewhere"}`, []string{"inside the project directory"}},
		{"not json", `project: demo`, []string{"invalid ship8t.json"}},
		{"too large", `{"ignore":["` + strings.Repeat("a", maxProjectFileBytes) + `"]}`, []string{"larger than 65536 bytes"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			writeProjectFile(t, directory, test.content)

			got, err := LoadProjectFile(directory)
			if err == nil {
				t.Fatalf("LoadProjectFile() = %+v, nil error, want an error", got)
			}
			for _, want := range append(test.wantInErr, "project directory", directory) {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("LoadProjectFile() error = %q, want it to contain %q", err, want)
				}
			}
			if strings.Contains(err.Error(), "s3cret") {
				t.Errorf("LoadProjectFile() error = %q, want it not to repeat the token", err)
			}
		})
	}
}
