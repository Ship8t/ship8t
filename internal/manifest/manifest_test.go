package manifest

import (
	"errors"
	"strings"
	"testing"
)

const validChecksum = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// validFile is one valid file entry that the cases below embed in a manifest.
const validFile = `{"path":"a.html","size":1,"sha256":"` + validChecksum + `"}`

func TestDecodeRejectsInvalidManifests(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"empty body", ``},
		{"malformed json", `{"manifest_version": 1,`},
		{"json array", `[]`},
		{"json null", `null`},
		{"unknown top-level field", `{"manifest_version":1,"files":[` + validFile + `],"extra":true}`},
		{"unknown file field", `{"manifest_version":1,"files":[{"path":"a.html","size":1,"sha256":"` + validChecksum + `","mode":420}]}`},
		{"unknown serve field", `{"manifest_version":1,"files":[` + validFile + `],"serve":{"spa_fallbak":true}}`},
		{"missing files", `{"manifest_version":1}`},
		{"empty files", `{"manifest_version":1,"files":[]}`},
		{"fractional size", `{"manifest_version":1,"files":[{"path":"a.html","size":1.5,"sha256":"` + validChecksum + `"}]}`},
		{"path traversal", `{"manifest_version":1,"files":[{"path":"../a.html","size":1,"sha256":"` + validChecksum + `"}]}`},
		{"trailing data", `{"manifest_version":1,"files":[` + validFile + `]} {}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Decode(strings.NewReader(test.body), DefaultLimits()); err == nil {
				t.Fatalf("Decode(%q) = nil error, want an error", test.body)
			}
		})
	}
}

func TestDecodeRejectsUnsupportedVersions(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"missing version", `{"files":[` + validFile + `]}`},
		{"version zero", `{"manifest_version":0,"files":[` + validFile + `]}`},
		{"future version", `{"manifest_version":2,"files":[` + validFile + `]}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Decode(strings.NewReader(test.body), DefaultLimits())
			if !errors.Is(err, ErrUnsupportedVersion) {
				t.Fatalf("Decode error = %v, want ErrUnsupportedVersion", err)
			}
		})
	}
}

func TestDecodeNamesTheFileWithTheInvalidPath(t *testing.T) {
	body := `{"manifest_version":1,"files":[
		{"path":"index.html","size":1,"sha256":"` + validChecksum + `"},
		{"path":"assets//app.js","size":1,"sha256":"` + validChecksum + `"}
	]}`

	_, err := Decode(strings.NewReader(body), DefaultLimits())
	if err == nil {
		t.Fatal("Decode returned nil, want an error")
	}
	if !strings.Contains(err.Error(), "files[1]") {
		t.Fatalf("error %q does not name files[1]", err)
	}
}

func TestDecodeReadsAValidManifest(t *testing.T) {
	body := `{
		"manifest_version": 1,
		"files": [
			{"path": "index.html", "size": 1234, "sha256": "` + validChecksum + `"},
			{"path": "assets/app.js", "size": 0, "sha256": "` + validChecksum + `"}
		],
		"serve": {"spa_fallback": true}
	}`

	got, err := Decode(strings.NewReader(body), DefaultLimits())
	if err != nil {
		t.Fatalf("Decode returned %v, want nil", err)
	}
	if got.ManifestVersion != 1 {
		t.Errorf("ManifestVersion = %d, want 1", got.ManifestVersion)
	}
	if len(got.Files) != 2 {
		t.Fatalf("len(Files) = %d, want 2", len(got.Files))
	}
	if want := (File{Path: "index.html", Size: 1234, SHA256: validChecksum}); got.Files[0] != want {
		t.Errorf("Files[0] = %+v, want %+v", got.Files[0], want)
	}
	if !got.Serve.SPAFallback {
		t.Error("Serve.SPAFallback = false, want true")
	}
}

func TestDecodeDefaultsServeOptionsToOff(t *testing.T) {
	body := `{"manifest_version":1,"files":[` + validFile + `]}`

	got, err := Decode(strings.NewReader(body), DefaultLimits())
	if err != nil {
		t.Fatalf("Decode returned %v, want nil", err)
	}
	if got.Serve.SPAFallback {
		t.Error("Serve.SPAFallback = true, want false")
	}
}

func TestDecodeRequiresNFCPaths(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"precomposed path", `caf\u00e9.html`, false},
		{"decomposed path", `cafe\u0301.html`, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := `{"manifest_version":1,"files":[{"path":"` + test.path + `","size":1,"sha256":"` + validChecksum + `"}]}`

			_, err := Decode(strings.NewReader(body), DefaultLimits())
			if gotErr := err != nil; gotErr != test.wantErr {
				t.Fatalf("Decode error = %v, want an error: %v", err, test.wantErr)
			}
			if test.wantErr && !strings.Contains(err.Error(), "NFC") {
				t.Fatalf("error %q does not name the NFC form", err)
			}
		})
	}
}

func TestDecodeRejectsPathsThatAreNotValidUTF8(t *testing.T) {
	// encoding/json replaces invalid bytes and lone surrogates with U+FFFD, so
	// without a check the client's name would silently change into another name.
	file := func(path string) string {
		return filesBody(`{"path":"` + path + `","size":1,"sha256":"` + validChecksum + `"}`)
	}

	tests := []struct {
		name string
		body string
	}{
		{"raw invalid byte", file("a\xff.html")},
		{"raw truncated sequence", file("a\xe2\x82.html")},
		{"lone high surrogate escape", file(`a\ud800.html`)},
		{"lone low surrogate escape", file(`a\udc00.html`)},
		{"lone surrogate in a later segment", file(`dir/a\ud800.html`)},
		{"high surrogate before a plain letter", file(`a\ud83dx.html`)},
		{"escaped replacement character", file(`a\ufffd.html`)},
		{"raw replacement character", file("a\ufffd.html")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Decode(strings.NewReader(test.body), DefaultLimits())
			if err == nil {
				t.Fatalf("Decode(%q) = nil error, want an error", test.body)
			}
			if !strings.Contains(err.Error(), "U+FFFD") {
				t.Errorf("error %q does not name U+FFFD", err)
			}
		})
	}
}

func TestDecodeAcceptsPathsWithValidSurrogatePairs(t *testing.T) {
	body := filesBody(`{"path":"a\ud83d\ude00.html","size":1,"sha256":"` + validChecksum + `"}`)

	got, err := Decode(strings.NewReader(body), DefaultLimits())
	if err != nil {
		t.Fatalf("Decode returned %v, want nil", err)
	}
	if want := "a\U0001f600.html"; got.Files[0].Path != want {
		t.Fatalf("Path = %q, want %q", got.Files[0].Path, want)
	}
}
