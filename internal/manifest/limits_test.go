package manifest

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
)

// manifestWithSizes returns a manifest with one file for each size.
func manifestWithSizes(sizes ...int64) *Manifest {
	files := make([]File, 0, len(sizes))
	for _, size := range sizes {
		files = append(files, File{Size: size})
	}
	return &Manifest{ManifestVersion: supportedVersion, Files: files}
}

// fileWithSize returns a valid file entry with its own path and the given size.
func fileWithSize(number int, size int64) string {
	return fmt.Sprintf(`{"path":"file%d.html","size":%d,"sha256":%q}`, number, size, validChecksum)
}

func TestDefaultLimitsMatchADR0004(t *testing.T) {
	want := Limits{
		MaxFiles:     10_000,
		MaxFileSize:  50 << 20,
		MaxTotalSize: 500 << 20,
	}

	if got := DefaultLimits(); got != want {
		t.Fatalf("DefaultLimits() = %+v, want %+v", got, want)
	}
}

func TestCheckLimitsRejectsManifestsOverALimit(t *testing.T) {
	limits := Limits{MaxFileSize: 100, MaxTotalSize: 250}

	tests := []struct {
		name     string
		manifest *Manifest
		want     []string
	}{
		{"one file over the size limit", manifestWithSizes(101), []string{"files[0]", "limit of 100 bytes", "size of one file"}},
		{"later file over the size limit", manifestWithSizes(1, 1, 101), []string{"files[2]", "limit of 100 bytes", "size of one file"}},
		{"files over the total size limit", manifestWithSizes(100, 100, 100), []string{"files[2]", "limit of 250 bytes", "total size"}},
		{"one byte over the total size limit", manifestWithSizes(100, 100, 51), []string{"files[2]", "limit of 250 bytes", "total size"}},
		{"huge file size", manifestWithSizes(math.MaxInt64), []string{"files[0]", "size of one file"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.manifest.checkLimits(limits)
			if !errors.Is(err, ErrLimitExceeded) {
				t.Fatalf("checkLimits error = %v, want ErrLimitExceeded", err)
			}
			for _, want := range test.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestCheckLimitsDoesNotOverflowTheTotalSize(t *testing.T) {
	limits := Limits{MaxFileSize: math.MaxInt64, MaxTotalSize: math.MaxInt64}

	err := manifestWithSizes(math.MaxInt64, math.MaxInt64).checkLimits(limits)
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("checkLimits error = %v, want ErrLimitExceeded", err)
	}
}

func TestCheckLimitsAcceptsManifestsAtTheLimits(t *testing.T) {
	limits := Limits{MaxFileSize: 100, MaxTotalSize: 250}

	tests := []struct {
		name     string
		manifest *Manifest
	}{
		{"no files", manifestWithSizes()},
		{"empty files", manifestWithSizes(0, 0)},
		{"exactly one file size", manifestWithSizes(100)},
		{"exactly the total size", manifestWithSizes(100, 100, 50)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.manifest.checkLimits(limits); err != nil {
				t.Fatalf("checkLimits returned %v, want nil", err)
			}
		})
	}
}

func TestDecodeRejectsFilesOverTheSizeLimits(t *testing.T) {
	limits := Limits{MaxFiles: 3, MaxFileSize: 100, MaxTotalSize: 150}

	tests := []struct {
		name string
		body string
		want string
	}{
		{"one file over the size limit", filesBody(fileWithSize(1, 101)), "size of one file"},
		{"files over the total size limit", filesBody(fileWithSize(1, 100), fileWithSize(2, 51)), "total size"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Decode(strings.NewReader(test.body), limits)
			if !errors.Is(err, ErrLimitExceeded) {
				t.Fatalf("Decode error = %v, want ErrLimitExceeded", err)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error %q does not name the limit %q", err, test.want)
			}
		})
	}
}
