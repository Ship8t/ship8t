package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// supportedVersion is the only manifest_version this package accepts.
const supportedVersion = 1

// ErrUnsupportedVersion marks a manifest whose manifest_version is missing or
// not supported, so the server can answer with unsupported_manifest_version.
var ErrUnsupportedVersion = errors.New("unsupported manifest version")

// Manifest lists the files of one deployment and how to serve them.
type Manifest struct {
	ManifestVersion int    `json:"manifest_version"`
	Files           []File `json:"files"`
	Serve           Serve  `json:"serve"`
}

// File describes one file of a deployment by path, size, and SHA-256 checksum.
type File struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Serve holds the serving options of a deployment.
type Serve struct {
	SPAFallback bool `json:"spa_fallback"`
}

// Decode reads one manifest from reader and validates it against the manifest
// rules of ADR 0004. The caller must cap the size of reader.
func Decode(reader io.Reader) (*Manifest, error) {
	decoder := json.NewDecoder(reader)
	// Reject unknown fields so a typo cannot silently change behavior.
	decoder.DisallowUnknownFields()

	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("decode manifest: unexpected data after the manifest")
	}
	if err := manifest.validate(); err != nil {
		return nil, err
	}
	return &manifest, nil
}

// validate checks the version, the file count, every file, and how the file
// paths relate to each other.
func (manifest *Manifest) validate() error {
	if manifest.ManifestVersion != supportedVersion {
		return fmt.Errorf("%w: got %d, want %d", ErrUnsupportedVersion, manifest.ManifestVersion, supportedVersion)
	}
	if len(manifest.Files) == 0 {
		return errors.New("invalid manifest: files must list at least one file")
	}
	for index, file := range manifest.Files {
		if err := file.validate(); err != nil {
			return fmt.Errorf("invalid manifest: files[%d]: %w", index, err)
		}
	}
	if err := checkPathCollisions(manifest.Files); err != nil {
		return fmt.Errorf("invalid manifest: %w", err)
	}
	return nil
}
