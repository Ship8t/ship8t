package scan

import (
	"errors"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ship8t/ship8t/internal/manifest"
)

const (
	// helloChecksum is the SHA-256 of the text "hello".
	helloChecksum = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	// emptyChecksum is the SHA-256 of no bytes.
	emptyChecksum = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

func TestDescribeHashesFiles(t *testing.T) {
	root := t.TempDir()
	writeFileContent(t, root, "index.html", "hello")
	writeFileContent(t, root, "assets/empty.txt", "")
	writeFileContent(t, root, "assets/hello.txt", "hello")

	files := []string{"assets/empty.txt", "assets/hello.txt", "index.html"}
	got, err := Describe(root, files, manifest.DefaultLimits())
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	want := []manifest.File{
		{Path: "assets/empty.txt", Size: 0, SHA256: emptyChecksum},
		{Path: "assets/hello.txt", Size: 5, SHA256: helloChecksum},
		{Path: "index.html", Size: 5, SHA256: helloChecksum},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Describe() = %v, want %v", got, want)
	}
}

func TestDescribeFollowsTheScanResult(t *testing.T) {
	root := t.TempDir()
	writeFileContent(t, root, "index.html", "hello")
	writeFileContent(t, root, "assets/app.js", "hello")
	writeFileContent(t, root, ".hidden", "hello")

	files, err := Scan(root, Options{})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	got, err := Describe(root, files, manifest.DefaultLimits())
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	var paths []string
	for _, file := range got {
		paths = append(paths, file.Path)
	}
	if want := []string{"assets/app.js", "index.html"}; !reflect.DeepEqual(paths, want) {
		t.Errorf("Describe() paths = %v, want %v", paths, want)
	}
}

func TestDescribeResultPassesManifestValidation(t *testing.T) {
	root := t.TempDir()
	writeFileContent(t, root, "index.html", "hello")

	files, err := Describe(root, []string{"index.html"}, manifest.DefaultLimits())
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	built := &manifest.Manifest{ManifestVersion: manifest.Version, Files: files}
	if err := built.Validate(manifest.DefaultLimits()); err != nil {
		t.Errorf("Validate() error = %v, want nil for the files that Describe returns", err)
	}
}

func TestDescribeRejectsPathsThatLeaveTheDirectory(t *testing.T) {
	parent := t.TempDir()
	if err := os.WriteFile(parent+"/outside.txt", []byte("secret"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	root := parent + "/site"
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatalf("create deploy directory: %v", err)
	}
	writeFile(t, root, "index.html")

	for _, file := range []string{"../outside.txt", parent + "/outside.txt", "assets/../../outside.txt"} {
		t.Run(file, func(t *testing.T) {
			got, err := Describe(root, []string{"index.html", file}, manifest.DefaultLimits())
			if err == nil {
				t.Fatalf("Describe() = %v, nil error, want an error", got)
			}
		})
	}
}

func TestDescribeRejectsASymlinkThatAppearedAfterTheScan(t *testing.T) {
	outside := t.TempDir()
	writeFile(t, outside, "secret.txt")

	tests := []struct {
		name   string
		target string
	}{
		{"symlink to a file in the directory", "index.html"},
		{"symlink to a file outside the directory", outside + "/secret.txt"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, "index.html")
			symlink(t, root, test.target, "link.html")

			got, err := Describe(root, []string{"index.html", "link.html"}, manifest.DefaultLimits())
			if err == nil {
				t.Fatalf("Describe() = %v, nil error, want an error", got)
			}
			for _, want := range []string{`"link.html"`, "not a regular file"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Describe() error = %q, want it to contain %q", err, want)
				}
			}
		})
	}
}

func TestDescribeRejectsDirectoriesAndMissingFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "assets/app.js")

	for _, file := range []string{"assets", "missing.html"} {
		t.Run(file, func(t *testing.T) {
			got, err := Describe(root, []string{file}, manifest.DefaultLimits())
			if err == nil {
				t.Fatalf("Describe() = %v, nil error, want an error", got)
			}
			if !strings.Contains(err.Error(), `"`+file+`"`) {
				t.Errorf("Describe() error = %q, want it to name %q", err, file)
			}
		})
	}
}

func TestDescribeRejectsListsThatBreakTheManifestRules(t *testing.T) {
	// A decomposed é is a file name that macOS can produce and that is not in NFC form.
	decomposed := "é.html"

	tests := []struct {
		name      string
		files     map[string]string
		list      []string
		wantInErr string
	}{
		{"no files", nil, nil, "at least one file"},
		{"name that is not in nfc form", map[string]string{decomposed: "hello"}, []string{decomposed}, "NFC"},
		{"path listed twice", map[string]string{"a.html": "x"}, []string{"a.html", "a.html"}, "duplicate"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			for path, content := range test.files {
				writeFileContent(t, root, path, content)
			}

			got, err := Describe(root, test.list, manifest.DefaultLimits())
			if err == nil {
				t.Fatalf("Describe() = %v, nil error, want an error", got)
			}
			if !strings.Contains(err.Error(), test.wantInErr) {
				t.Errorf("Describe() error = %q, want it to contain %q", err, test.wantInErr)
			}
		})
	}
}

func TestDescribeEnforcesLimits(t *testing.T) {
	tests := []struct {
		name      string
		files     map[string]string
		list      []string
		limits    manifest.Limits
		wantInErr string
	}{
		{
			name:      "more files than the limit",
			files:     map[string]string{"a.html": "x", "b.html": "x", "c.html": "x"},
			list:      []string{"a.html", "b.html", "c.html"},
			limits:    manifest.Limits{MaxFiles: 2, MaxFileSize: 100, MaxTotalSize: 100},
			wantInErr: "limit of 2 files",
		},
		{
			name:      "file over the size limit",
			files:     map[string]string{"a.html": strings.Repeat("x", 11)},
			list:      []string{"a.html"},
			limits:    manifest.Limits{MaxFiles: 10, MaxFileSize: 10, MaxTotalSize: 100},
			wantInErr: "size of one file",
		},
		{
			name:      "files over the total size limit",
			files:     map[string]string{"a.html": strings.Repeat("x", 6), "b.html": strings.Repeat("x", 6)},
			list:      []string{"a.html", "b.html"},
			limits:    manifest.Limits{MaxFiles: 10, MaxFileSize: 10, MaxTotalSize: 11},
			wantInErr: "total size",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			for path, content := range test.files {
				writeFileContent(t, root, path, content)
			}

			got, err := Describe(root, test.list, test.limits)
			if !errors.Is(err, manifest.ErrLimitExceeded) {
				t.Fatalf("Describe() = %v, error %v, want manifest.ErrLimitExceeded", got, err)
			}
			if !strings.Contains(err.Error(), test.wantInErr) {
				t.Errorf("Describe() error = %q, want it to contain %q", err, test.wantInErr)
			}
		})
	}
}

func TestDescribeAcceptsFilesAtTheLimits(t *testing.T) {
	root := t.TempDir()
	writeFileContent(t, root, "a.html", strings.Repeat("x", 10))
	writeFileContent(t, root, "b.html", strings.Repeat("x", 10))

	limits := manifest.Limits{MaxFiles: 2, MaxFileSize: 10, MaxTotalSize: 20}
	if _, err := Describe(root, []string{"a.html", "b.html"}, limits); err != nil {
		t.Errorf("Describe() error = %v, want nil at exactly the limits", err)
	}
}

func TestHashFileReadsOneByteMoreThanTheLimit(t *testing.T) {
	root := t.TempDir()
	writeFileContent(t, root, "big.bin", strings.Repeat("x", 100))
	opened, err := os.OpenRoot(root)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	defer opened.Close()

	size, _, err := hashFile(opened, "big.bin", 10)
	if err != nil {
		t.Fatalf("hashFile() error = %v", err)
	}
	// One byte past the limit is enough for Validate to reject the file.
	if size != 11 {
		t.Errorf("hashFile() size = %d, want 11, one byte more than the limit of 10", size)
	}
}

func TestHashFileHandlesTheLargestLimit(t *testing.T) {
	root := t.TempDir()
	writeFileContent(t, root, "hello.txt", "hello")
	opened, err := os.OpenRoot(root)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	defer opened.Close()

	// A server can report any limit. Adding one to the largest value must not
	// turn the limit negative and hash nothing.
	size, checksum, err := hashFile(opened, "hello.txt", math.MaxInt64)
	if err != nil {
		t.Fatalf("hashFile() error = %v", err)
	}
	if size != 5 || checksum != helloChecksum {
		t.Errorf("hashFile() = %d, %q, want 5, %q", size, checksum, helloChecksum)
	}
}
