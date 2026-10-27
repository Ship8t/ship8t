package manifest

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// filesBody returns a manifest body whose files array holds the given JSON
// elements.
func filesBody(elements ...string) string {
	return `{"manifest_version":1,"files":[` + strings.Join(elements, ",") + `]}`
}

// distinctFile returns a valid file entry with its own path.
func distinctFile(number int) string {
	return fmt.Sprintf(`{"path":"file%d.html","size":1,"sha256":%q}`, number, validChecksum)
}

func TestDecodeRejectsMoreFilesThanTheLimit(t *testing.T) {
	limits := Limits{MaxFiles: 3, MaxFileSize: 100, MaxTotalSize: 1000}

	tests := []struct {
		name string
		body string
	}{
		{"one file over the limit", filesBody(distinctFile(1), distinctFile(2), distinctFile(3), distinctFile(4))},
		// Empty objects are invalid files, so only a count check that runs first can reject them as too many.
		{"empty objects over the limit", filesBody("{}", "{}", "{}", "{}")},
		// A malformed element past the limit must not be decoded, so the count error wins.
		{"wrong type past the limit", filesBody(distinctFile(1), distinctFile(2), distinctFile(3), `{"path":5}`)},
		{"unknown field past the limit", filesBody(distinctFile(1), distinctFile(2), distinctFile(3), `{"mode":420}`)},
		{"many files over the limit", filesBody(strings.Split(strings.Repeat("{},", 99)+"{}", ",")...)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Decode(strings.NewReader(test.body), limits)
			if !errors.Is(err, ErrLimitExceeded) {
				t.Fatalf("Decode error = %v, want ErrLimitExceeded", err)
			}
			if !strings.Contains(err.Error(), "limit of 3 files per deployment") {
				t.Fatalf("error %q does not name the file count limit", err)
			}
		})
	}
}

func TestDecodeAcceptsExactlyTheFileLimit(t *testing.T) {
	limits := Limits{MaxFiles: 3, MaxFileSize: 100, MaxTotalSize: 1000}

	got, err := Decode(strings.NewReader(filesBody(distinctFile(1), distinctFile(2), distinctFile(3))), limits)
	if err != nil {
		t.Fatalf("Decode returned %v, want nil", err)
	}
	if len(got.Files) != 3 {
		t.Fatalf("len(Files) = %d, want 3", len(got.Files))
	}
}

func TestDecodeStillRejectsUnknownFieldsInsideTheFilesArray(t *testing.T) {
	body := filesBody(`{"path":"a.html","size":1,"sha256":"` + validChecksum + `","mode":420}`)

	if _, err := Decode(strings.NewReader(body), DefaultLimits()); err == nil {
		t.Fatal("Decode returned nil, want an error")
	}
}

func TestDecodeRejectsAFilesFieldThatIsNotAnArray(t *testing.T) {
	tests := []struct {
		name  string
		files string
	}{
		{"null", `null`},
		{"number", `5`},
		{"string", `"index.html"`},
		{"object", `{"path":"a.html"}`},
		{"true", `true`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := `{"manifest_version":1,"files":` + test.files + `}`
			if _, err := Decode(strings.NewReader(body), DefaultLimits()); err == nil {
				t.Fatalf("Decode(%s) = nil error, want an error", body)
			}
		})
	}
}

func TestDecodeKeepsOnlyTheLastFilesFieldWhenItIsRepeated(t *testing.T) {
	limits := Limits{MaxFiles: 2, MaxFileSize: 100, MaxTotalSize: 1000}
	body := `{"manifest_version":1,"files":[` + distinctFile(1) + `,` + distinctFile(2) + `],"files":[` + distinctFile(3) + `]}`

	got, err := Decode(strings.NewReader(body), limits)
	if err != nil {
		t.Fatalf("Decode returned %v, want nil", err)
	}
	if len(got.Files) != 1 || got.Files[0].Path != "file3.html" {
		t.Fatalf("Files = %+v, want only file3.html", got.Files)
	}
}

func TestDecodeDoesNotAllocateAFileForEveryElementOfAHugeArray(t *testing.T) {
	// About 2.8 million empty objects fit in the 8 MiB body cap. Decoding them
	// all into File structs would allocate more than 100 MiB.
	body := `{"manifest_version":1,"files":[` + strings.Repeat("{},", (MaxBodyBytes-60)/3) + `{}]}`

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err := Decode(strings.NewReader(body), DefaultLimits())
	runtime.ReadMemStats(&after)

	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("Decode error = %v, want ErrLimitExceeded", err)
	}
	const maxAllocatedBytes = 100 << 20
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > maxAllocatedBytes {
		t.Fatalf("Decode allocated %d bytes, want at most %d", allocated, maxAllocatedBytes)
	}
}
