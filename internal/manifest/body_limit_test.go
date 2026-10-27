package manifest

import (
	"errors"
	"strings"
	"testing"
)

// endlessSpaces is a reader that never ends, to prove Decode stops reading.
type endlessSpaces struct{}

// Read fills buffer with spaces and never reports the end of the input.
func (endlessSpaces) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = ' '
	}
	return len(buffer), nil
}

// paddedManifest returns a valid manifest followed by spaces, so the body is
// exactly size bytes long.
func paddedManifest(t *testing.T, size int) string {
	t.Helper()
	body := `{"manifest_version":1,"files":[` + validFile + `]}`
	if len(body) > size {
		t.Fatalf("manifest is %d bytes, larger than the requested %d bytes", len(body), size)
	}
	return body + strings.Repeat(" ", size-len(body))
}

func TestDecodeRejectsBodiesOverTheLimit(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"valid manifest one byte over", paddedManifest(t, MaxBodyBytes+1)},
		{"valid manifest far over", paddedManifest(t, 2*MaxBodyBytes)},
		{"truncated json over the limit", `{"manifest_version":1,"files":[` + strings.Repeat(" ", MaxBodyBytes)},
		{"giant json string over the limit", `"` + strings.Repeat("a", MaxBodyBytes)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Decode(strings.NewReader(test.body), DefaultLimits())
			if !errors.Is(err, ErrLimitExceeded) {
				t.Fatalf("Decode error = %v, want ErrLimitExceeded", err)
			}
			if !strings.Contains(err.Error(), "8388608 bytes") {
				t.Fatalf("error %q does not name the 8388608 byte limit", err)
			}
		})
	}
}

func TestDecodeStopsReadingAnEndlessBody(t *testing.T) {
	_, err := Decode(endlessSpaces{}, DefaultLimits())
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("Decode error = %v, want ErrLimitExceeded", err)
	}
}

func TestDecodeAcceptsABodyAtTheLimit(t *testing.T) {
	body := paddedManifest(t, MaxBodyBytes)

	if _, err := Decode(strings.NewReader(body), DefaultLimits()); err != nil {
		t.Fatalf("Decode returned %v, want nil", err)
	}
}
