package manifest

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

// singleFileManifest returns a manifest body with one file, so each case
// below changes only the size or checksum it tests.
func singleFileManifest(t *testing.T, size, checksum string) string {
	t.Helper()
	// Encode the checksum as JSON, because %q writes escapes JSON rejects, such as \x00.
	encodedChecksum, err := json.Marshal(checksum)
	if err != nil {
		t.Fatalf("encode checksum: %v", err)
	}
	return fmt.Sprintf(`{"manifest_version":1,"files":[{"path":"a.html","size":%s,"sha256":%s}]}`, size, encodedChecksum)
}

func TestDecodeRejectsInvalidFileSizes(t *testing.T) {
	tests := []struct {
		name string
		size string
	}{
		{"negative size", "-1"},
		{"large negative size", "-9223372036854775808"},
		{"size above the int64 range", "9223372036854775808"},
		{"size as a string", `"10"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := singleFileManifest(t, test.size, validChecksum)
			if _, err := Decode(strings.NewReader(body), DefaultLimits()); err == nil {
				t.Fatalf("Decode(%s) = nil error, want an error", body)
			}
		})
	}
}

func TestDecodeRejectsInvalidChecksums(t *testing.T) {
	tests := []struct {
		name     string
		checksum string
	}{
		{"empty checksum", ""},
		{"uppercase checksum", strings.ToUpper(validChecksum)},
		{"one uppercase character", "E" + validChecksum[1:]},
		{"short checksum", validChecksum[:63]},
		{"long checksum", validChecksum + "0"},
		{"non-hex character", "g" + validChecksum[1:]},
		{"checksum with a prefix", "sha256:" + validChecksum[7:]},
		{"checksum with a space", " " + validChecksum[1:]},
		{"checksum with a nul character", "\x00" + validChecksum[1:]},
		{"checksum with a non-ascii digit", "٣" + validChecksum[2:]},
		// Characters just below and above the lower end of each range, which no other case covers.
		{"character before zero", "/" + validChecksum[1:]},
		{"character after nine", ":" + validChecksum[1:]},
		{"character before a", "`" + validChecksum[1:]},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := singleFileManifest(t, "1", test.checksum)
			if _, err := Decode(strings.NewReader(body), DefaultLimits()); err == nil {
				t.Fatalf("Decode(%s) = nil error, want an error", body)
			}
		})
	}
}

func TestDecodeRejectsAMissingChecksum(t *testing.T) {
	body := `{"manifest_version":1,"files":[{"path":"a.html","size":1}]}`
	if _, err := Decode(strings.NewReader(body), DefaultLimits()); err == nil {
		t.Fatal("Decode returned nil, want an error")
	}
}

func TestDecodeAcceptsValidSizesAndChecksums(t *testing.T) {
	// Lift the size limits, because this test is about the size and checksum format.
	limits := Limits{MaxFiles: 1, MaxFileSize: math.MaxInt64, MaxTotalSize: math.MaxInt64}

	tests := []struct {
		name     string
		size     string
		checksum string
	}{
		{"empty file", "0", validChecksum},
		{"largest size", "9223372036854775807", validChecksum},
		{"checksum of only digits", "1", strings.Repeat("0123456789", 6) + "0123"},
		{"checksum of only letters", "1", strings.Repeat("abcdef", 10) + "abcd"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := singleFileManifest(t, test.size, test.checksum)
			if _, err := Decode(strings.NewReader(body), limits); err != nil {
				t.Fatalf("Decode(%s) = %v, want nil", body, err)
			}
		})
	}
}

func TestDecodeDoesNotEchoTheChecksum(t *testing.T) {
	checksum := strings.Repeat("Z", 4096)

	_, err := Decode(strings.NewReader(singleFileManifest(t, "1", checksum)), DefaultLimits())
	if err == nil {
		t.Fatal("Decode returned nil, want an error")
	}
	if strings.Contains(err.Error(), checksum) {
		t.Fatal("error repeats the invalid checksum")
	}
}

func TestDecodeNamesTheFileWithTheInvalidChecksum(t *testing.T) {
	body := `{"manifest_version":1,"files":[
		{"path":"index.html","size":1,"sha256":"` + validChecksum + `"},
		{"path":"app.js","size":1,"sha256":"abc"}
	]}`

	_, err := Decode(strings.NewReader(body), DefaultLimits())
	if err == nil {
		t.Fatal("Decode returned nil, want an error")
	}
	if !strings.Contains(err.Error(), "files[1]") {
		t.Fatalf("error %q does not name files[1]", err)
	}
}

func TestDecodeRequiresASizeForEveryFile(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{"missing size", `{"path":"b.html","sha256":"` + validChecksum + `"}`},
		{"null size", `{"path":"b.html","size":null,"sha256":"` + validChecksum + `"}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := filesBody(validFile, test.file)

			_, err := Decode(strings.NewReader(body), DefaultLimits())
			if err == nil {
				t.Fatalf("Decode(%s) = nil error, want an error", body)
			}
			for _, want := range []string{"files[1]", "size is required"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestDecodeAcceptsAnExplicitZeroSize(t *testing.T) {
	body := filesBody(`{"path":"empty.txt","size":0,"sha256":"` + validChecksum + `"}`)

	got, err := Decode(strings.NewReader(body), DefaultLimits())
	if err != nil {
		t.Fatalf("Decode returned %v, want nil", err)
	}
	if got.Files[0].Size != 0 {
		t.Fatalf("Size = %d, want 0", got.Files[0].Size)
	}
}

func TestDecodeReportsAMissingSizeBeforeALaterLimitError(t *testing.T) {
	// A missing size is a shape error, so it wins over the count limit that a later element would hit.
	limits := Limits{MaxFiles: 1, MaxFileSize: 100, MaxTotalSize: 100}
	body := filesBody(`{"path":"a.html","sha256":"`+validChecksum+`"}`, validFile)

	_, err := Decode(strings.NewReader(body), limits)
	if err == nil || !strings.Contains(err.Error(), "size is required") {
		t.Fatalf("Decode error = %v, want a missing size error", err)
	}
}
