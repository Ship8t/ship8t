package manifest

import (
	"errors"
	"math"
	"strings"
	"testing"
)

// otherChecksum is a second well-formed checksum, so a test can tell two
// contents apart.
const otherChecksum = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

func TestCheckChecksumSizesRejectsOneChecksumWithTwoSizes(t *testing.T) {
	tests := []struct {
		name  string
		files []File
		want  string
	}{
		{
			name:  "second file has another size",
			files: []File{{Path: "a.html", Size: 5, SHA256: validChecksum}, {Path: "b.html", Size: 9, SHA256: validChecksum}},
			want:  "files[1] has size 9 but files[0] has the same sha256 and size 5",
		},
		{
			name:  "same pair in the other order",
			files: []File{{Path: "a.html", Size: 9, SHA256: validChecksum}, {Path: "b.html", Size: 5, SHA256: validChecksum}},
			want:  "files[1] has size 5 but files[0] has the same sha256 and size 9",
		},
		{
			name: "third file against the first",
			files: []File{
				{Path: "a.html", Size: 5, SHA256: validChecksum},
				{Path: "b.html", Size: 7, SHA256: otherChecksum},
				{Path: "c.html", Size: 9, SHA256: validChecksum},
			},
			want: "files[2] has size 9 but files[0] has the same sha256 and size 5",
		},
		{
			name: "third file after two that agree",
			files: []File{
				{Path: "a.html", Size: 5, SHA256: validChecksum},
				{Path: "b.html", Size: 5, SHA256: validChecksum},
				{Path: "c.html", Size: 9, SHA256: validChecksum},
			},
			want: "files[2] has size 9 but files[0] has the same sha256 and size 5",
		},
		{
			name:  "empty file and a file with content",
			files: []File{{Path: "a.html", Size: 0, SHA256: validChecksum}, {Path: "b.html", Size: 1, SHA256: validChecksum}},
			want:  "files[1] has size 1 but files[0] has the same sha256 and size 0",
		},
		{
			name:  "largest size and one byte",
			files: []File{{Path: "a.html", Size: math.MaxInt64, SHA256: otherChecksum}, {Path: "b.html", Size: 1, SHA256: otherChecksum}},
			want:  "files[1] has size 1 but files[0] has the same sha256 and size 9223372036854775807",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := checkChecksumSizes(test.files)

			if err == nil || err.Error() != test.want {
				t.Errorf("checkChecksumSizes() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCheckChecksumSizesAcceptsConsistentFiles(t *testing.T) {
	tests := []struct {
		name  string
		files []File
	}{
		{"one file", []File{{Path: "a.html", Size: 5, SHA256: validChecksum}}},
		{"two files with one checksum and one size", []File{{Path: "a.html", Size: 5, SHA256: validChecksum}, {Path: "b.html", Size: 5, SHA256: validChecksum}}},
		{"two empty files", []File{{Path: "a.txt", Size: 0, SHA256: validChecksum}, {Path: "b.txt", Size: 0, SHA256: validChecksum}}},
		{"two checksums with two sizes", []File{{Path: "a.html", Size: 5, SHA256: validChecksum}, {Path: "b.html", Size: 9, SHA256: otherChecksum}}},
		{"two checksums with one size", []File{{Path: "a.html", Size: 5, SHA256: validChecksum}, {Path: "b.html", Size: 5, SHA256: otherChecksum}}},
		{"one checksum three times with one size", []File{
			{Path: "a.html", Size: 7, SHA256: otherChecksum},
			{Path: "b.html", Size: 7, SHA256: otherChecksum},
			{Path: "c.html", Size: 7, SHA256: otherChecksum},
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := checkChecksumSizes(test.files); err != nil {
				t.Errorf("checkChecksumSizes() error = %v, want nil", err)
			}
		})
	}
}

func TestValidateRejectsOneChecksumWithTwoSizes(t *testing.T) {
	manifest := &Manifest{ManifestVersion: supportedVersion, Files: []File{
		{Path: "a.html", Size: 5, SHA256: validChecksum},
		{Path: "b.html", Size: 9, SHA256: validChecksum},
	}}

	err := manifest.Validate(DefaultLimits())

	want := "invalid manifest: files[1] has size 9 but files[0] has the same sha256 and size 5"
	if err == nil || err.Error() != want {
		t.Errorf("Validate() error = %v, want %q", err, want)
	}
	if errors.Is(err, ErrLimitExceeded) || errors.Is(err, ErrUnsupportedVersion) {
		t.Errorf("Validate() error = %v, want an error that is neither a limit nor a version error", err)
	}
}

func TestDecodeRejectsOneChecksumWithTwoSizes(t *testing.T) {
	body := filesBody(
		`{"path":"a.html","size":5,"sha256":"`+validChecksum+`"}`,
		`{"path":"b.html","size":9,"sha256":"`+validChecksum+`"}`,
	)

	_, err := Decode(strings.NewReader(body), DefaultLimits())

	want := "invalid manifest: files[1] has size 9 but files[0] has the same sha256 and size 5"
	if err == nil || err.Error() != want {
		t.Errorf("Decode() error = %v, want %q", err, want)
	}
}

func TestValidateAcceptsOneChecksumWithOneSize(t *testing.T) {
	manifest := &Manifest{ManifestVersion: supportedVersion, Files: []File{
		{Path: "a.html", Size: 5, SHA256: validChecksum},
		{Path: "b.html", Size: 5, SHA256: validChecksum},
	}}

	if err := manifest.Validate(DefaultLimits()); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidateReportsAPathCollisionBeforeAChecksumSize(t *testing.T) {
	manifest := &Manifest{ManifestVersion: supportedVersion, Files: []File{
		{Path: "A.html", Size: 5, SHA256: validChecksum},
		{Path: "a.html", Size: 9, SHA256: validChecksum},
	}}

	err := manifest.Validate(DefaultLimits())

	if err == nil || !strings.Contains(err.Error(), "differs only by case") {
		t.Errorf("Validate() error = %v, want the path collision", err)
	}
}
