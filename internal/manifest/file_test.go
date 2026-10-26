package manifest

import (
	"encoding/json"
	"fmt"
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
			if _, err := Decode(strings.NewReader(body)); err == nil {
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := singleFileManifest(t, "1", test.checksum)
			if _, err := Decode(strings.NewReader(body)); err == nil {
				t.Fatalf("Decode(%s) = nil error, want an error", body)
			}
		})
	}
}

func TestDecodeRejectsAMissingChecksum(t *testing.T) {
	body := `{"manifest_version":1,"files":[{"path":"a.html","size":1}]}`
	if _, err := Decode(strings.NewReader(body)); err == nil {
		t.Fatal("Decode returned nil, want an error")
	}
}

func TestDecodeAcceptsValidSizesAndChecksums(t *testing.T) {
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
			if _, err := Decode(strings.NewReader(body)); err != nil {
				t.Fatalf("Decode(%s) = %v, want nil", body, err)
			}
		})
	}
}

func TestDecodeDoesNotEchoTheChecksum(t *testing.T) {
	checksum := strings.Repeat("Z", 4096)

	_, err := Decode(strings.NewReader(singleFileManifest(t, "1", checksum)))
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

	_, err := Decode(strings.NewReader(body))
	if err == nil {
		t.Fatal("Decode returned nil, want an error")
	}
	if !strings.Contains(err.Error(), "files[1]") {
		t.Fatalf("error %q does not name files[1]", err)
	}
}
