package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// fuzzLimits keeps the limits small, so short inputs reach the limit checks.
var fuzzLimits = Limits{MaxFiles: 4, MaxFileSize: 1000, MaxTotalSize: 2000}

// fuzzFile returns the JSON of one file entry with the checksum of an empty
// file.
func fuzzFile(path string, size int) string {
	return fmt.Sprintf(`{"path":%s,"size":%d,"sha256":%q}`, path, size, validChecksum)
}

// fuzzManifest returns the JSON of a manifest of version 1 with the file
// entries.
func fuzzManifest(files ...string) string {
	return `{"manifest_version":1,"files":[` + strings.Join(files, ",") + `]}`
}

// fuzzSeeds returns a valid manifest and one input for each rule that the
// decoder enforces, so the fuzzer starts from both sides of every rule.
func fuzzSeeds() []string {
	valid := fuzzManifest(fuzzFile(`"index.html"`, 10), fuzzFile(`"assets/app.js"`, 20))
	return []string{
		valid,
		`{"manifest_version":1,"files":[` + fuzzFile(`"index.html"`, 0) + `],"serve":{"spa_fallback":true}}`,
		// Path rules.
		fuzzManifest(fuzzFile(`"../etc/passwd"`, 1)),
		fuzzManifest(fuzzFile(`"/etc/passwd"`, 1)),
		fuzzManifest(fuzzFile(`"a\\b"`, 1)),
		fuzzManifest(fuzzFile(`"é.html"`, 1)),
		fuzzManifest(fuzzFile(`"a\u0000b"`, 1)),
		fuzzManifest(fuzzFile(`"a�b"`, 1)),
		fuzzManifest(fuzzFile(`"nul.txt"`, 1)),
		fuzzManifest(fuzzFile(`"a//b"`, 1)),
		fuzzManifest(fuzzFile(`"a/./b"`, 1)),
		// Collisions.
		fuzzManifest(fuzzFile(`"a.html"`, 1), fuzzFile(`"a.html"`, 1)),
		fuzzManifest(fuzzFile(`"A.html"`, 1), fuzzFile(`"a.html"`, 1)),
		fuzzManifest(fuzzFile(`"docs"`, 1), fuzzFile(`"docs/a.html"`, 1)),
		// Field rules.
		`{"manifest_version":1,"files":[{"path":"a","size":1,"sha256":"` + validChecksum + `","mode":420}]}`,
		`{"manifest_version":1,"files":[{"Path":"a","size":1,"sha256":"` + validChecksum + `"}]}`,
		`{"manifest_version":1,"manifest_version":1,"files":[` + fuzzFile(`"a"`, 1) + `]}`,
		`{"manifest_version":1,"files":[{"path":"a","sha256":"` + validChecksum + `"}]}`,
		`{"manifest_version":1,"files":[{"path":"a","size":-1,"sha256":"` + validChecksum + `"}]}`,
		`{"manifest_version":1,"files":[{"path":"a","size":1,"sha256":"abc"}]}`,
		valid + ` trailing`,
		// Version and shape.
		`{"manifest_version":2,"files":[` + fuzzFile(`"a"`, 1) + `]}`,
		`{"manifest_version":1,"files":[]}`,
		`{"manifest_version":1,"files":null}`,
		`[]`,
		`null`,
		``,
		// Limits.
		fuzzManifest(fuzzFile(`"a"`, 1), fuzzFile(`"b"`, 1), fuzzFile(`"c"`, 1), fuzzFile(`"d"`, 1), fuzzFile(`"e"`, 1)),
		fuzzManifest(fuzzFile(`"a"`, 1001)),
		fuzzManifest(fuzzFile(`"a"`, 900), fuzzFile(`"b"`, 900), fuzzFile(`"c"`, 900)),
	}
}

// checkAcceptedManifest fails the test unless an accepted manifest holds the
// security rules of ADR 0004. It spells the rules out apart from the decoder,
// so a bug in the decoder cannot hide in the check.
func checkAcceptedManifest(t *testing.T, accepted *Manifest) {
	t.Helper()
	if accepted.ManifestVersion != 1 {
		t.Errorf("accepted manifest_version %d, want 1", accepted.ManifestVersion)
	}
	if len(accepted.Files) < 1 || len(accepted.Files) > fuzzLimits.MaxFiles {
		t.Errorf("accepted %d files, want 1 to %d", len(accepted.Files), fuzzLimits.MaxFiles)
	}

	var total int64
	seen := make(map[string]bool)
	for index, file := range accepted.Files {
		if seen[file.Path] {
			t.Errorf("files[%d] repeats the path %q", index, file.Path)
		}
		seen[file.Path] = true

		if file.Size < 0 || file.Size > fuzzLimits.MaxFileSize {
			t.Errorf("files[%d] has size %d outside 0 to %d", index, file.Size, fuzzLimits.MaxFileSize)
		}
		total += file.Size

		if len(file.SHA256) != 64 || strings.Trim(file.SHA256, "0123456789abcdef") != "" {
			t.Errorf("files[%d] has the checksum %q, want 64 lowercase hexadecimal characters", index, file.SHA256)
		}
		checkAcceptedPath(t, index, file.Path)
	}
	if total > fuzzLimits.MaxTotalSize {
		t.Errorf("accepted a total size of %d, over the limit of %d", total, fuzzLimits.MaxTotalSize)
	}
}

// checkAcceptedPath fails the test when an accepted path could escape the
// release directory or break on a filesystem.
func checkAcceptedPath(t *testing.T, index int, path string) {
	t.Helper()
	if path == "" || len(path) > 1024 {
		t.Errorf("files[%d] has a path of %d bytes, want 1 to 1024", index, len(path))
	}
	if !utf8.ValidString(path) || strings.ContainsRune(path, utf8.RuneError) {
		t.Errorf("files[%d] path %q is not clean UTF-8", index, path)
	}
	if !norm.NFC.IsNormalString(path) {
		t.Errorf("files[%d] path %q is not in NFC form", index, path)
	}
	if strings.HasPrefix(path, "/") || strings.Contains(path, `\`) {
		t.Errorf("files[%d] path %q is absolute or holds a backslash", index, path)
	}
	if strings.ContainsFunc(path, unicode.IsControl) {
		t.Errorf("files[%d] path %q holds a control character", index, path)
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "" || segment == "." || segment == ".." || len(segment) > 255 {
			t.Errorf("files[%d] path %q has the segment %q", index, path, segment)
		}
	}
}

func FuzzDecode(f *testing.F) {
	for _, seed := range fuzzSeeds() {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, body []byte) {
		accepted, err := Decode(bytes.NewReader(body), fuzzLimits)
		if err != nil {
			if accepted != nil {
				t.Fatalf("Decode() returned a manifest and the error %v", err)
			}
			return
		}
		checkAcceptedManifest(t, accepted)

		// Decode what the client would send for this manifest. It must give the
		// same manifest, or the client and the server could disagree.
		encoded, err := json.Marshal(accepted)
		if err != nil {
			t.Fatalf("encode accepted manifest: %v", err)
		}
		again, err := Decode(bytes.NewReader(encoded), fuzzLimits)
		if err != nil {
			t.Fatalf("Decode() rejected the encoding of an accepted manifest %s: %v", encoded, err)
		}
		if !reflect.DeepEqual(accepted, again) {
			t.Errorf("Decode() of the encoding = %+v, want %+v", again, accepted)
		}
	})
}

func TestFuzzSeedsReachBothSidesOfTheDecoder(t *testing.T) {
	var accepted, rejected int
	for _, seed := range fuzzSeeds() {
		if _, err := Decode(strings.NewReader(seed), fuzzLimits); err == nil {
			accepted++
		} else {
			rejected++
		}
	}
	// Two seeds are valid. Every other seed breaks one rule.
	if accepted != 2 {
		t.Errorf("accepted %d seeds, want the 2 valid seeds", accepted)
	}
	if rejected < 20 {
		t.Errorf("rejected %d seeds, want one for each rule", rejected)
	}
}
