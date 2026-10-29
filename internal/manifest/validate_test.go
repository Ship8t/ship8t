package manifest

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// builtManifest returns a manifest of version 1 with one valid file for each path.
func builtManifest(paths ...string) *Manifest {
	return &Manifest{ManifestVersion: supportedVersion, Files: filesWithPaths(paths...)}
}

func TestValidateRejectsInvalidManifests(t *testing.T) {
	limits := Limits{MaxFiles: 3, MaxFileSize: 100, MaxTotalSize: 250}
	badChecksum := builtManifest("a.html")
	badChecksum.Files[0].SHA256 = "abc"
	negativeSize := builtManifest("a.html")
	negativeSize.Files[0].Size = -1
	bigFile := builtManifest("a.html")
	bigFile.Files[0].Size = 101
	bigTotal := builtManifest("a.html", "b.html", "c.html")
	for index := range bigTotal.Files {
		bigTotal.Files[index].Size = 100
	}

	tests := []struct {
		name      string
		manifest  *Manifest
		wantIs    error
		wantInErr string
	}{
		{"unsupported version", &Manifest{ManifestVersion: 2, Files: filesWithPaths("a.html")}, ErrUnsupportedVersion, "got 2"},
		{"missing version", &Manifest{Files: filesWithPaths("a.html")}, ErrUnsupportedVersion, "got 0"},
		{"no files", &Manifest{ManifestVersion: supportedVersion}, nil, "at least one file"},
		{"path traversal", builtManifest("../secret.txt"), nil, "files[0]"},
		{"absolute path", builtManifest("/etc/passwd"), nil, "files[0]"},
		{"path that is not in nfc form", builtManifest("index.html", "é.html"), nil, "files[1]"},
		{"invalid checksum", badChecksum, nil, "invalid sha256"},
		{"negative size", negativeSize, nil, "invalid size"},
		{"paths that differ only by case", builtManifest("A.html", "a.html"), nil, "differs only by case"},
		{"file as a directory of another file", builtManifest("docs", "docs/a.html"), nil, "docs"},
		{"file over the size limit", bigFile, ErrLimitExceeded, "size of one file"},
		{"files over the total size limit", bigTotal, ErrLimitExceeded, "total size"},
		{"more files than the limit", builtManifest("a.html", "b.html", "c.html", "d.html"), ErrLimitExceeded, "limit of 3 files"},
		{"limit before the rules for each file", builtManifest("../a", "../b", "../c", "../d"), ErrLimitExceeded, "limit of 3 files"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.manifest.Validate(limits)
			if err == nil {
				t.Fatal("Validate() = nil error, want an error")
			}
			if test.wantIs != nil && !errors.Is(err, test.wantIs) {
				t.Errorf("Validate() error = %v, want it to wrap %v", err, test.wantIs)
			}
			if test.wantIs == nil && (errors.Is(err, ErrLimitExceeded) || errors.Is(err, ErrUnsupportedVersion)) {
				t.Errorf("Validate() error = %v, want a rule error, not a limit or version error", err)
			}
			if !strings.Contains(err.Error(), test.wantInErr) {
				t.Errorf("Validate() error = %q, want it to contain %q", err, test.wantInErr)
			}
		})
	}
}

func TestValidateAcceptsManifestsWithinTheLimits(t *testing.T) {
	limits := Limits{MaxFiles: 2, MaxFileSize: 100, MaxTotalSize: 200}
	manifest := builtManifest("index.html", "assets/app.js")
	for index := range manifest.Files {
		manifest.Files[index].Size = 100
	}

	if err := manifest.Validate(limits); err != nil {
		t.Errorf("Validate() error = %v, want nil at exactly the limits", err)
	}
	if err := builtManifest("index.html", "assets/app.js").Validate(DefaultLimits()); err != nil {
		t.Errorf("Validate() error = %v, want nil with the default limits", err)
	}
}

func TestValidateReportsWhatDecodeReports(t *testing.T) {
	limits := Limits{MaxFiles: 10, MaxFileSize: 100, MaxTotalSize: 150}
	bigFile := builtManifest("a.html")
	bigFile.Files[0].Size = 101
	bigTotal := builtManifest("a.html", "b.html")
	for index := range bigTotal.Files {
		bigTotal.Files[index].Size = 100
	}

	tests := []struct {
		name     string
		manifest *Manifest
	}{
		{"unsupported version", &Manifest{ManifestVersion: 2, Files: filesWithPaths("a.html")}},
		{"path traversal", builtManifest("../secret.txt")},
		{"paths that differ only by case", builtManifest("A.html", "a.html")},
		{"file over the size limit", bigFile},
		{"files over the total size limit", bigTotal},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, err := json.Marshal(test.manifest)
			if err != nil {
				t.Fatalf("encode manifest: %v", err)
			}
			_, decodeErr := Decode(strings.NewReader(string(body)), limits)
			validateErr := test.manifest.Validate(limits)
			if decodeErr == nil || validateErr == nil {
				t.Fatalf("Decode() error = %v, Validate() error = %v, want both to fail", decodeErr, validateErr)
			}
			if decodeErr.Error() != validateErr.Error() {
				t.Errorf("Decode() error = %q, Validate() error = %q, want the same message", decodeErr, validateErr)
			}
		})
	}
}

func TestVersionIsTheVersionThatValidateAccepts(t *testing.T) {
	manifest := &Manifest{ManifestVersion: Version, Files: filesWithPaths("index.html")}
	if err := manifest.Validate(DefaultLimits()); err != nil {
		t.Errorf("Validate() error = %v, want nil for a manifest of Version %d", err, Version)
	}
}
