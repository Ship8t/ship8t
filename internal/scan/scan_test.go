package scan

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeFile creates a file with the given slash-separated path under directory.
func writeFile(t *testing.T, directory, path string) {
	t.Helper()
	full := filepath.Join(directory, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("create directory for %s: %v", path, err)
	}
	if err := os.WriteFile(full, []byte("content"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// symlink creates a symlink at path under directory that points to target.
func symlink(t *testing.T, directory, target, path string) {
	t.Helper()
	full := filepath.Join(directory, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("create directory for %s: %v", path, err)
	}
	if err := os.Symlink(target, full); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
}

func TestScanRejectsSymlinks(t *testing.T) {
	outside := t.TempDir()
	writeFile(t, outside, "secret.txt")

	tests := []struct {
		name      string
		build     func(t *testing.T, root string)
		wantInErr string
	}{
		{
			name: "symlink to a file inside the root",
			build: func(t *testing.T, root string) {
				writeFile(t, root, "index.html")
				symlink(t, root, "index.html", "link.html")
			},
			wantInErr: "link.html",
		},
		{
			name: "symlink to a directory inside the root",
			build: func(t *testing.T, root string) {
				writeFile(t, root, "assets/app.js")
				symlink(t, root, "assets", "static")
			},
			wantInErr: "static",
		},
		{
			name: "symlink to a file outside the root",
			build: func(t *testing.T, root string) {
				symlink(t, root, filepath.Join(outside, "secret.txt"), "leak.txt")
			},
			wantInErr: "leak.txt",
		},
		{
			name: "symlink to a directory outside the root",
			build: func(t *testing.T, root string) {
				symlink(t, root, outside, "leak")
			},
			wantInErr: "leak",
		},
		{
			name: "nested symlink",
			build: func(t *testing.T, root string) {
				writeFile(t, root, "docs/guide/index.html")
				symlink(t, root, "../../index.html", "docs/guide/link.html")
			},
			wantInErr: "docs/guide/link.html",
		},
		{
			name: "dangling symlink",
			build: func(t *testing.T, root string) {
				symlink(t, root, "missing.html", "dangling.html")
			},
			wantInErr: "dangling.html",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			test.build(t, root)

			files, err := Scan(root)
			if err == nil {
				t.Fatalf("Scan() = %v, nil error, want an error", files)
			}
			if !strings.Contains(err.Error(), test.wantInErr) {
				t.Errorf("Scan() error = %q, want it to name %q", err, test.wantInErr)
			}
			if !strings.Contains(err.Error(), "symlink") {
				t.Errorf("Scan() error = %q, want it to mention a symlink", err)
			}
		})
	}
}

func TestScanListsRegularFiles(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  []string
	}{
		{"empty directory", nil, []string{}},
		{"one file", []string{"index.html"}, []string{"index.html"}},
		{
			name:  "nested directories",
			files: []string{"index.html", "assets/css/site.css", "assets/app.js"},
			want:  []string{"assets/app.js", "assets/css/site.css", "index.html"},
		},
		{
			name:  "paths are sorted by byte order",
			files: []string{"b.html", "Z.html", "a/z.html", "a.html"},
			want:  []string{"Z.html", "a.html", "a/z.html", "b.html"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			for _, path := range test.files {
				writeFile(t, root, path)
			}

			got, err := Scan(root)
			if err != nil {
				t.Fatalf("Scan() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("Scan() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestScanFailsOnMissingDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if files, err := Scan(missing); err == nil {
		t.Fatalf("Scan(%q) = %v, nil error, want an error", missing, files)
	}
}

func TestScanFailsOnFileAsRoot(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "index.html")
	path := filepath.Join(root, "index.html")
	if files, err := Scan(path); err == nil {
		t.Fatalf("Scan(%q) = %v, nil error, want an error", path, files)
	}
}
