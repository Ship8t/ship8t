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
	writeFileContent(t, directory, path, "content")
}

// writeFileContent creates a file with the given content at the slash-separated
// path under directory.
func writeFileContent(t *testing.T, directory, path, content string) {
	t.Helper()
	full := filepath.Join(directory, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("create directory for %s: %v", path, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
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

// assertScan creates the files under a new directory, scans it with options,
// and fails the test unless the scan succeeds and lists exactly want.
func assertScan(t *testing.T, files []string, options Options, want []string) {
	t.Helper()
	root := t.TempDir()
	for _, path := range files {
		writeFile(t, root, path)
	}

	got, err := Scan(root, options)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Scan() = %v, want %v", got, want)
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

			files, err := Scan(root, Options{})
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
			assertScan(t, test.files, Options{}, test.want)
		})
	}
}

func TestScanSkipsDefaultIgnores(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  []string
	}{
		{
			name:  "git directory",
			files: []string{"index.html", ".git/config", ".git/objects/ab/cdef"},
			want:  []string{"index.html"},
		},
		{
			name:  "node_modules at any depth",
			files: []string{"index.html", "node_modules/lib/index.js", "docs/node_modules/lib/index.js"},
			want:  []string{"index.html"},
		},
		{
			name:  "ds store at any depth",
			files: []string{"index.html", ".DS_Store", "assets/.DS_Store"},
			want:  []string{"index.html"},
		},
		{
			name:  "ship8t config",
			files: []string{"index.html", "ship8t.json"},
			want:  []string{"index.html"},
		},
		{
			name:  "dotfiles and dot directories",
			files: []string{"index.html", ".env", ".cache/data", "assets/.hidden"},
			want:  []string{"index.html"},
		},
		{
			name:  "well-known directory is kept",
			files: []string{"index.html", ".well-known/security.txt"},
			want:  []string{".well-known/security.txt", "index.html"},
		},
		{
			name:  "dotfile inside well-known is skipped",
			files: []string{".well-known/security.txt", ".well-known/.hidden"},
			want:  []string{".well-known/security.txt"},
		},
		{
			name:  "node_modules file is kept",
			files: []string{"docs/node_modules"},
			want:  []string{"docs/node_modules"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertScan(t, test.files, Options{}, test.want)
		})
	}
}

func TestScanSkipsSymlinksUnderIgnoredPaths(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "index.html")
	writeFile(t, root, "node_modules/lib/index.js")
	symlink(t, root, "../lib/index.js", "node_modules/.bin/tool")
	symlink(t, root, "index.html", ".hidden-link")

	got, err := Scan(root, Options{})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if want := []string{"index.html"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Scan() = %v, want %v", got, want)
	}
}

func TestScanRejectsSymlinkBesideIgnoredPaths(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "index.html")
	writeFile(t, root, ".git/config")
	symlink(t, root, "index.html", "link.html")

	if files, err := Scan(root, Options{}); err == nil {
		t.Fatalf("Scan() = %v, nil error, want an error for link.html", files)
	}
}

func TestScanSkipsIgnorePatterns(t *testing.T) {
	tests := []struct {
		name   string
		files  []string
		ignore []string
		want   []string
	}{
		{
			name:   "extension at any depth",
			files:  []string{"index.html", "app.js.map", "assets/app.js", "assets/app.js.map"},
			ignore: []string{"**/*.map"},
			want:   []string{"assets/app.js", "index.html"},
		},
		{
			name:   "extension at the root only",
			files:  []string{"index.html", "app.js.map", "assets/app.js.map"},
			ignore: []string{"*.map"},
			want:   []string{"assets/app.js.map", "index.html"},
		},
		{
			name:   "directory pattern removes the root directory only",
			files:  []string{"index.html", "build/out.js", "docs/build"},
			ignore: []string{"build/"},
			want:   []string{"docs/build", "index.html"},
		},
		{
			name:   "double star removes a whole tree",
			files:  []string{"index.html", "drafts/a.html", "drafts/deep/b.html"},
			ignore: []string{"drafts/**"},
			want:   []string{"index.html"},
		},
		{
			name:   "directory name without a slash removes the tree",
			files:  []string{"index.html", "drafts/a.html", "drafts/deep/b.html"},
			ignore: []string{"drafts"},
			want:   []string{"index.html"},
		},
		{
			name:   "one exact file",
			files:  []string{"docs/draft.html", "docs/index.html"},
			ignore: []string{"docs/draft.html"},
			want:   []string{"docs/index.html"},
		},
		{
			name:   "several patterns",
			files:  []string{"a.tmp", "b.bak", "c.html"},
			ignore: []string{"*.tmp", "*.bak"},
			want:   []string{"c.html"},
		},
		{
			name:   "patterns add to the default rules",
			files:  []string{"index.html", ".hidden", "notes.txt"},
			ignore: []string{"notes.txt"},
			want:   []string{"index.html"},
		},
		{
			name:   "pattern that matches nothing",
			files:  []string{"index.html", "assets/app.js"},
			ignore: []string{"missing/**"},
			want:   []string{"assets/app.js", "index.html"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertScan(t, test.files, Options{Ignore: test.ignore}, test.want)
		})
	}
}

func TestScanSkipsSymlinkThatIgnorePatternNames(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "index.html")
	symlink(t, root, "index.html", "link.html")

	got, err := Scan(root, Options{Ignore: []string{"link.html"}})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if want := []string{"index.html"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Scan() = %v, want %v", got, want)
	}
}

func TestScanRejectsSymlinkThatIgnorePatternsMiss(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "index.html")
	symlink(t, root, "index.html", "link.html")

	files, err := Scan(root, Options{Ignore: []string{"*.map"}})
	if err == nil {
		t.Fatalf("Scan() = %v, nil error, want an error for link.html", files)
	}
	if !strings.Contains(err.Error(), "link.html") {
		t.Errorf("Scan() error = %q, want it to name link.html", err)
	}
}

func TestScanRejectsInvalidIgnorePattern(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "index.html")

	files, err := Scan(root, Options{Ignore: []string{"*.map", "!index.html"}})
	if err == nil {
		t.Fatalf("Scan() = %v, nil error, want an error", files)
	}
	for _, want := range []string{"ignore", "negation"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Scan() error = %q, want it to contain %q", err, want)
		}
	}
}

func TestScanAppliesIncludePatterns(t *testing.T) {
	tests := []struct {
		name    string
		files   []string
		include []string
		ignore  []string
		want    []string
	}{
		{
			name:    "adds back one file of a skipped directory",
			files:   []string{"index.html", ".github/pages.html", ".github/other.html"},
			include: []string{".github/pages.html"},
			want:    []string{".github/pages.html", "index.html"},
		},
		{
			name:    "adds back a dotfile at the root",
			files:   []string{"index.html", ".htaccess", ".env"},
			include: []string{".htaccess"},
			want:    []string{".htaccess", "index.html"},
		},
		{
			name:    "adds back a dotfile only where the pattern says",
			files:   []string{"index.html", ".htaccess", "docs/.htaccess"},
			include: []string{".htaccess"},
			want:    []string{".htaccess", "index.html"},
		},
		{
			name:    "adds back a skipped directory and what is below it",
			files:   []string{"index.html", ".cache/a.txt", ".cache/deep/b.txt"},
			include: []string{".cache"},
			want:    []string{".cache/a.txt", ".cache/deep/b.txt", "index.html"},
		},
		{
			name:    "adds back a directory with a trailing slash pattern",
			files:   []string{"index.html", ".cache/a.txt"},
			include: []string{".cache/"},
			want:    []string{".cache/a.txt", "index.html"},
		},
		{
			name:    "adds back the dotfiles below an included directory",
			files:   []string{"index.html", ".cache/a.txt", ".cache/.hidden"},
			include: []string{".cache"},
			want:    []string{".cache/.hidden", ".cache/a.txt", "index.html"},
		},
		{
			name:    "adds back a nested directory of a skipped directory",
			files:   []string{".github/pages/a.html", ".github/pages/deep/b.html", ".github/other.html"},
			include: []string{".github/pages"},
			want:    []string{".github/pages/a.html", ".github/pages/deep/b.html"},
		},
		{
			name:    "adds back a file below node_modules",
			files:   []string{"index.html", "node_modules/lib/index.js", "node_modules/lib/skip.js"},
			include: []string{"node_modules/lib/index.js"},
			want:    []string{"index.html", "node_modules/lib/index.js"},
		},
		{
			name:    "adds back files with a double star",
			files:   []string{"index.html", ".cache/a.txt", ".cache/.hidden"},
			include: []string{".cache/**"},
			want:    []string{".cache/.hidden", ".cache/a.txt", "index.html"},
		},
		{
			name:    "pattern that matches nothing changes nothing",
			files:   []string{"index.html", ".hidden", ".cache/a.txt"},
			include: []string{"missing/**"},
			want:    []string{"index.html"},
		},
		{
			name:    "pattern does not add back a file that no default rule removed",
			files:   []string{"index.html", "a.txt"},
			include: []string{"a.txt"},
			want:    []string{"a.txt", "index.html"},
		},
		{
			name:    "ignore wins over include",
			files:   []string{".cache/a.txt", ".cache/b.txt"},
			include: []string{".cache"},
			ignore:  []string{"**/b.txt"},
			want:    []string{".cache/a.txt"},
		},
		{
			name:    "ignore on the directory wins over include",
			files:   []string{"index.html", ".cache/a.txt"},
			include: []string{".cache/a.txt"},
			ignore:  []string{".cache"},
			want:    []string{"index.html"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertScan(t, test.files, Options{Include: test.include, Ignore: test.ignore}, test.want)
		})
	}
}

func TestScanSkipsSymlinkInDirectoryThatIncludeOpens(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".github/pages.html")
	symlink(t, root, "pages.html", ".github/link")

	got, err := Scan(root, Options{Include: []string{".github/pages.html"}})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if want := []string{".github/pages.html"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Scan() = %v, want %v", got, want)
	}
}

func TestScanRejectsSymlinkThatIncludeAddsBack(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "index.html")
	symlink(t, root, "index.html", ".link")

	files, err := Scan(root, Options{Include: []string{".link"}})
	if err == nil {
		t.Fatalf("Scan() = %v, nil error, want an error", files)
	}
	if !strings.Contains(err.Error(), ".link") || !strings.Contains(err.Error(), "symlink") {
		t.Errorf("Scan() error = %q, want it to name .link and mention a symlink", err)
	}
}

func TestScanRejectsInvalidIncludePattern(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "index.html")

	files, err := Scan(root, Options{Include: []string{"/abs"}})
	if err == nil {
		t.Fatalf("Scan() = %v, nil error, want an error", files)
	}
	for _, want := range []string{"include", "relative"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Scan() error = %q, want it to contain %q", err, want)
		}
	}
}

func TestScanFailsOnMissingDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if files, err := Scan(missing, Options{}); err == nil {
		t.Fatalf("Scan(%q) = %v, nil error, want an error", missing, files)
	}
}

func TestScanFailsOnFileAsRoot(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "index.html")
	path := filepath.Join(root, "index.html")
	if files, err := Scan(path, Options{}); err == nil {
		t.Fatalf("Scan(%q) = %v, nil error, want an error", path, files)
	}
}
