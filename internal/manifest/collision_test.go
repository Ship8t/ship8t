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
		// Pairs below are one file on a case-insensitive APFS volume, checked by hand.
		{"long s and s", []string{"a/s.html", "a/\u017f.html"}},
		{"sigma and final sigma", []string{"a/\u03c3.html", "a/\u03c2.html"}},
		{"micro sign and greek mu", []string{"a/\u00b5.html", "a/\u03bc.html"}},
		{"theta and theta symbol", []string{"a/\u03b8.html", "a/\u03d1.html"}},
		{"cyrillic ve and rounded ve", []string{"a/\u0432.html", "a/\u1c80.html"}},
		{"iota and combining ypogegrammeni", []string{"a/\u03b9.html", "a/\u0345.html"}},
		{"sharp s and ss", []string{"a/\u00df.html", "a/ss.html"}},
		{"capital sharp s and ss", []string{"a/\u1e9e.html", "a/ss.html"}},
		{"fi ligature and fi", []string{"a/\ufb01le.html", "a/file.html"}},
		{"n with apostrophe and apostrophe n", []string{"a/\u0149.html", "a/\u02bcn.html"}},
		{"dotted capital i and i with a dot above", []string{"a/\u0130.html", "a/i\u0307.html"}},
		{"long s file inside a directory of s", []string{"x\u017f", "xs/y.html"}},
		{"sharp s file inside a directory of ss", []string{"xss/y.html", "x\u00df"}},
		// The directory side uses the unusual spelling, so the directory pass must fold too.
		{"directory spelled with capitals", []string{"docs", "Docs/a.html"}},
		{"directory spelled with long s", []string{"xs", "x\u017f/y.html"}},
		{"directory spelled with sharp s", []string{"xss", "x\u00df/y.html"}},
		// The x/text fold tables leave Cherokee unfolded, but APFS treats these as one file.
		{"cherokee letter and its lowercase", []string{"a/\u13a0.html", "a/\uab70.html"}},
		{"cherokee small letter and its other small form", []string{"a/\u13f5.html", "a/\u13fd.html"}},
		{"cherokee file inside a directory of its lowercase", []string{"\u13a0", "\uab70/f.html"}},
		// Both are NFC, and only a normalization step makes their folded forms equal.
		{"dotted capital i with a mark below and its decomposed form", []string{"a/i\u032f\u0307.html", "a/\u0130\u032f.html"}},
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
		// Pairs below stay two files on a case-insensitive APFS volume, checked by hand.
		{"dotless i and i", []string{"a/\u0131.html", "a/i.html"}},
		{"sharp s and s", []string{"a/\u00df.html", "a/s.html"}},
		{"fi ligature and f", []string{"a/\ufb01.html", "a/f.html"}},
		{"long s and z", []string{"a/\u017f.html", "a/z.html"}},
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

	if _, err := Decode(strings.NewReader(body), DefaultLimits()); err == nil {
		t.Fatal("Decode returned nil, want an error")
	}
}
