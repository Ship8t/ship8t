package manifest

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// lowChecksum sorts before otherChecksum and validChecksum, so a test can tell a
// sorted list from the order of the files.
const lowChecksum = "00000000000000000000000000000000000000000000000000000000000000ff"

func TestBlobsListsEachDistinctChecksumOnce(t *testing.T) {
	tests := []struct {
		name  string
		files []File
		want  []Blob
	}{
		{
			name:  "one file",
			files: []File{{Path: "a.html", Size: 5, SHA256: validChecksum}},
			want:  []Blob{{SHA256: validChecksum, Size: 5}},
		},
		{
			name: "two files with one content",
			files: []File{
				{Path: "a.html", Size: 5, SHA256: validChecksum},
				{Path: "copy/a.html", Size: 5, SHA256: validChecksum},
			},
			want: []Blob{{SHA256: validChecksum, Size: 5}},
		},
		{
			name: "three files with two contents",
			files: []File{
				{Path: "a.html", Size: 5, SHA256: otherChecksum},
				{Path: "b.html", Size: 9, SHA256: validChecksum},
				{Path: "c.html", Size: 5, SHA256: otherChecksum},
			},
			want: []Blob{{SHA256: otherChecksum, Size: 5}, {SHA256: validChecksum, Size: 9}},
		},
		{
			name: "two empty files and one file with content",
			files: []File{
				{Path: "a.txt", Size: 0, SHA256: validChecksum},
				{Path: "b.txt", Size: 7, SHA256: otherChecksum},
				{Path: "c.txt", Size: 0, SHA256: validChecksum},
			},
			want: []Blob{{SHA256: otherChecksum, Size: 7}, {SHA256: validChecksum, Size: 0}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := &Manifest{ManifestVersion: supportedVersion, Files: test.files}

			got := manifest.Blobs()

			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("Blobs() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestBlobsSortsByChecksumWhateverTheFileOrder(t *testing.T) {
	// The checksums are written out in sorted order, so a change of the order
	// rule is a decision here.
	sorted := []Blob{{SHA256: lowChecksum, Size: 9}, {SHA256: otherChecksum, Size: 3}, {SHA256: validChecksum, Size: 5}}
	orders := [][]int{{0, 1, 2}, {2, 1, 0}, {1, 2, 0}, {2, 0, 1}}
	for _, order := range orders {
		files := make([]File, 0, len(order))
		for _, index := range order {
			files = append(files, File{Path: "file" + string(rune('a'+index)) + ".html", Size: sorted[index].Size, SHA256: sorted[index].SHA256})
		}
		manifest := &Manifest{ManifestVersion: supportedVersion, Files: files}

		got := manifest.Blobs()

		if !reflect.DeepEqual(got, sorted) {
			t.Errorf("Blobs() for the file order %v = %+v, want %+v", order, got, sorted)
		}
	}
}

func TestBlobsLeavesTheManifestAlone(t *testing.T) {
	files := []File{
		{Path: "b.html", Size: 9, SHA256: validChecksum},
		{Path: "a.html", Size: 5, SHA256: otherChecksum},
		{Path: "c.html", Size: 9, SHA256: validChecksum},
	}
	manifest := &Manifest{ManifestVersion: supportedVersion, Files: append([]File(nil), files...)}

	manifest.Blobs()

	if !reflect.DeepEqual(manifest.Files, files) {
		t.Errorf("Files after Blobs() = %+v, want %+v", manifest.Files, files)
	}
}

func TestBlobsOfADecodedManifest(t *testing.T) {
	body := filesBody(
		`{"path":"index.html","size":1234,"sha256":"`+validChecksum+`"}`,
		`{"path":"assets/app.js","size":5,"sha256":"`+otherChecksum+`"}`,
		`{"path":"assets/copy.js","size":5,"sha256":"`+otherChecksum+`"}`,
	)
	decoded, err := Decode(strings.NewReader(body), DefaultLimits())
	if err != nil {
		t.Fatalf("Decode() error = %v, want nil", err)
	}

	got := decoded.Blobs()

	want := []Blob{{SHA256: otherChecksum, Size: 5}, {SHA256: validChecksum, Size: 1234}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Blobs() = %+v, want %+v", got, want)
	}
}

func TestBlobEncodesWithTheNamesOfADR0004(t *testing.T) {
	encoded, err := json.Marshal([]Blob{{SHA256: otherChecksum, Size: 5}, {SHA256: validChecksum, Size: 0}})
	if err != nil {
		t.Fatalf("Marshal() error = %v, want nil", err)
	}

	want := `[{"sha256":"` + otherChecksum + `","size":5},{"sha256":"` + validChecksum + `","size":0}]`
	if string(encoded) != want {
		t.Errorf("encoded blobs = %s, want %s", encoded, want)
	}
}
