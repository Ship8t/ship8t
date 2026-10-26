package manifest

import (
	"fmt"
	"strings"
	"testing"
)

// filesWithPaths returns one file entry for each path.
func filesWithPaths(paths ...string) []File {
	files := make([]File, 0, len(paths))
	for _, path := range paths {
		files = append(files, File{Path: path, Size: 1, SHA256: validChecksum})
	}
	return files
}

func TestCheckPathCollisionsRejectsCollidingPaths(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
	}{
		{"exact duplicate", []string{"index.html", "index.html"}},
		{"exact duplicate after other files", []string{"a.html", "b.html", "a.html"}},
		{"case-only difference", []string{"index.html", "INDEX.HTML"}},
		{"case-only difference in a directory", []string{"docs/a.html", "Docs/a.html"}},
		{"case-only difference in a non-ascii name", []string{"café.html", "CAFÉ.html"}},
		{"file before its directory", []string{"docs", "docs/a.html"}},
		{"file after its directory", []string{"docs/a.html", "docs"}},
		{"file inside a deep directory", []string{"a/b/c.html", "a/b"}},
		{"file at the top of a deep directory", []string{"a", "a/b/c/d.html"}},
		{"file and directory that differ by case", []string{"Docs", "docs/a.html"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := checkPathCollisions(filesWithPaths(test.paths...)); err == nil {
				t.Fatalf("checkPathCollisions(%q) = nil, want an error", test.paths)
			}
		})
	}
}

func TestCheckPathCollisionsAcceptsDistinctPaths(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
	}{
		{"no files", nil},
		{"one file", []string{"index.html"}},
		{"files in the same directory", []string{"docs/a.html", "docs/b.html"}},
		{"same name in different directories", []string{"a/index.html", "b/index.html"}},
		{"file that shares a name prefix with a directory", []string{"docs", "docs2/a.html"}},
		{"file with an extension next to a directory", []string{"about.html", "about/team.html"}},
		{"directory name that extends a file name", []string{"a", "ab/c.html"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := checkPathCollisions(filesWithPaths(test.paths...)); err != nil {
				t.Fatalf("checkPathCollisions(%q) = %v, want nil", test.paths, err)
			}
		})
	}
}

func TestCheckPathCollisionsAcceptsManyDistinctPaths(t *testing.T) {
	paths := make([]string, 0, 1000)
	for number := 0; number < 1000; number++ {
		paths = append(paths, fmt.Sprintf("assets/%d/file.js", number))
	}

	if err := checkPathCollisions(filesWithPaths(paths...)); err != nil {
		t.Fatalf("checkPathCollisions returned %v, want nil", err)
	}
}

func TestCheckPathCollisionsNamesBothFiles(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		want  []string
	}{
		{"duplicate", []string{"a.html", "b.html", "a.html"}, []string{"files[2]", "files[0]", "duplicate"}},
		{"case", []string{"a.html", "A.html"}, []string{"files[1]", "files[0]", "case"}},
		{"directory", []string{"docs/a.html", "docs"}, []string{"files[1]", "files[0]", "directory"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := checkPathCollisions(filesWithPaths(test.paths...))
			if err == nil {
				t.Fatal("checkPathCollisions returned nil, want an error")
			}
			for _, want := range test.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestDecodeRejectsCollidingPaths(t *testing.T) {
	body := `{"manifest_version":1,"files":[
		{"path":"docs","size":1,"sha256":"` + validChecksum + `"},
		{"path":"docs/a.html","size":1,"sha256":"` + validChecksum + `"}
	]}`

	if _, err := Decode(strings.NewReader(body)); err == nil {
		t.Fatal("Decode returned nil, want an error")
	}
}
