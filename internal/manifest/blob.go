package manifest

import (
	"slices"
	"strings"
)

// Blob is one distinct content of a deployment, named by its checksum and its
// size, in the form that ADR 0004 gives the entries of upload.missing.
type Blob struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Blobs returns each distinct content of the manifest once, sorted by checksum,
// so the answer does not depend on the order of the files. Files with the same
// checksum share one blob, and an empty file has a blob like any other. It
// expects a manifest that passed validation, which gives one size to each
// checksum.
func (manifest *Manifest) Blobs() []Blob {
	seen := make(map[string]bool, len(manifest.Files))
	blobs := make([]Blob, 0, len(manifest.Files))
	for _, file := range manifest.Files {
		if seen[file.SHA256] {
			continue
		}
		seen[file.SHA256] = true
		blobs = append(blobs, Blob{SHA256: file.SHA256, Size: file.Size})
	}
	slices.SortFunc(blobs, func(left, right Blob) int {
		return strings.Compare(left.SHA256, right.SHA256)
	})
	return blobs
}
