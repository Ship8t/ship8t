package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeUserFile creates config.json with the given content and mode in
// directory. It sets the mode after the write, so the umask cannot change it.
func writeUserFile(t *testing.T, directory, content string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(directory, "config.json")
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write config.json: %v", err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("set the mode of config.json: %v", err)
	}
}

func TestLoadUserFileReadsTheFile(t *testing.T) {
	directory := t.TempDir()
	writeUserFile(t, directory, `{"server":"https://deploy.example.com","token":"s3cret-value"}`, 0o600)

	got, err := LoadUserFile(directory)
	if err != nil {
		t.Fatalf("LoadUserFile() error = %v", err)
	}
	want := UserFile{Server: "https://deploy.example.com", Token: "s3cret-value"}
	if got != want {
		t.Errorf("LoadUserFile() = %+v, want %+v", got, want)
	}
}

func TestLoadUserFileTreatsAMissingFileAsNoSettings(t *testing.T) {
	directory := t.TempDir()
	// Other files must not stand in for config.json.
	if err := os.WriteFile(filepath.Join(directory, "other.json"), []byte(`{"token":"x"}`), 0o600); err != nil {
		t.Fatalf("write other.json: %v", err)
	}

	got, err := LoadUserFile(directory)
	if err != nil {
		t.Fatalf("LoadUserFile() error = %v, want no error for a missing file", err)
	}
	if got != (UserFile{}) {
		t.Errorf("LoadUserFile() = %+v, want no settings", got)
	}
}

func TestLoadUserFileTreatsAMissingDirectoryAsNoSettings(t *testing.T) {
	// The ship8t directory is created by the user, so a fresh machine has none.
	directory := filepath.Join(t.TempDir(), "missing", "ship8t")

	got, err := LoadUserFile(directory)
	if err != nil {
		t.Fatalf("LoadUserFile() error = %v, want no error for a missing directory", err)
	}
	if got != (UserFile{}) {
		t.Errorf("LoadUserFile() = %+v, want no settings", got)
	}
}

func TestLoadUserFileFailsOnABadDirectory(t *testing.T) {
	notADirectory := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(notADirectory, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tests := []struct {
		name      string
		directory string
	}{
		// An empty path must fail, because OpenRoot reports it as a missing file and
		// the loader would hide a caller that forgot to resolve the directory.
		{"empty path", ""},
		{"file instead of a directory", notADirectory},
		{"file in the place of a parent", filepath.Join(notADirectory, "ship8t")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := LoadUserFile(test.directory)
			if err == nil {
				t.Fatalf("LoadUserFile(%q) = %+v, nil error, want an error", test.directory, got)
			}
			if !strings.Contains(err.Error(), "user config directory") {
				t.Errorf("LoadUserFile(%q) error = %q, want it to name the user config directory", test.directory, err)
			}
		})
	}
}

func TestLoadUserFileRefusesAnythingButARegularFile(t *testing.T) {
	outside := t.TempDir()
	writeUserFile(t, outside, `{"token":"stolen-token"}`, 0o600)

	tests := []struct {
		name      string
		build     func(t *testing.T, directory string)
		wantInErr string
	}{
		{"directory", func(t *testing.T, directory string) {
			if err := os.Mkdir(filepath.Join(directory, "config.json"), 0o700); err != nil {
				t.Fatalf("create directory: %v", err)
			}
		}, "not a regular file"},
		{"symlink to a file in the directory", func(t *testing.T, directory string) {
			if err := os.WriteFile(filepath.Join(directory, "real.json"), []byte(`{"token":"real"}`), 0o600); err != nil {
				t.Fatalf("write real.json: %v", err)
			}
			symlinkOrSkip(t, "real.json", filepath.Join(directory, "config.json"))
		}, "refused on purpose"},
		{"symlink to a file outside the directory", func(t *testing.T, directory string) {
			symlinkOrSkip(t, filepath.Join(outside, "config.json"), filepath.Join(directory, "config.json"))
		}, "refused on purpose"},
		{"dangling symlink", func(t *testing.T, directory string) {
			symlinkOrSkip(t, "missing.json", filepath.Join(directory, "config.json"))
		}, "refused on purpose"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			test.build(t, directory)

			got, err := LoadUserFile(directory)
			if err == nil {
				t.Fatalf("LoadUserFile() = %+v, nil error, want an error", got)
			}
			if !strings.Contains(err.Error(), test.wantInErr) {
				t.Errorf("LoadUserFile() error = %q, want it to contain %q", err, test.wantInErr)
			}
			if got != (UserFile{}) {
				t.Errorf("LoadUserFile() = %+v, want the zero value with an error", got)
			}
		})
	}
}

func TestLoadUserFileReportsInvalidContent(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantInErr []string
	}{
		{"unknown field", `{"project":"x"}`, []string{"unknown field"}},
		{"bad server", `{"server":"ftp://example.com"}`, []string{"https://"}},
		{"not json", `server: x`, []string{"invalid user config"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			writeUserFile(t, directory, test.content, 0o600)

			got, err := LoadUserFile(directory)
			if err == nil {
				t.Fatalf("LoadUserFile() = %+v, nil error, want an error", got)
			}
			for _, want := range append(test.wantInErr, "user config directory") {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("LoadUserFile() error = %q, want it to contain %q", err, want)
				}
			}
			if got != (UserFile{}) {
				t.Errorf("LoadUserFile() = %+v, want the zero value with an error", got)
			}
		})
	}
}

func TestChmodCommand(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		want   string
		wantOK bool
	}{
		{"plain path", "/home/user/.config/ship8t/config.json", "chmod 600 '/home/user/.config/ship8t/config.json'", true},
		{"space", "/home/my user/config.json", "chmod 600 '/home/my user/config.json'", true},
		{"single quote", "/home/o'brien/config.json", `chmod 600 '/home/o'\''brien/config.json'`, true},
		{"only a quote", "'", `chmod 600 ''\'''`, true},
		{"shell syntax stays literal", "/a/$(id);`id`/*/b", "chmod 600 '/a/$(id);`id`/*/b'", true},
		{"non-ascii letters", "/home/josé/config.json", "chmod 600 '/home/josé/config.json'", true},
		{"newline", "/a\nb/config.json", "", false},
		{"carriage return", "/a\rb/config.json", "", false},
		{"tab", "/a\tb/config.json", "", false},
		{"escape", "/a\x1b[31mb/config.json", "", false},
		{"null byte", "/a\x00b/config.json", "", false},
		{"delete", "/a\x7fb/config.json", "", false},
		{"c1 control", "/a\u0085b/config.json", "", false},
		{"right to left override", "/a\u202eb/config.json", "", false},
		{"no-break space", "/a\u00a0b/config.json", "", false},
		{"invalid utf-8", "/a\xffb/config.json", "", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := chmodCommand(test.path)
			if ok != test.wantOK || got != test.want {
				t.Errorf("chmodCommand(%q) = %q, %v, want %q, %v", test.path, got, ok, test.want, test.wantOK)
			}
		})
	}
}
