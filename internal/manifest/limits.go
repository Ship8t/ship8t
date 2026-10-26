package manifest

import (
	"errors"
	"fmt"
)

// ErrLimitExceeded marks a manifest that goes over a deployment limit, so the
// server can answer with limit_exceeded.
var ErrLimitExceeded = errors.New("limit exceeded")

// Limits holds the size limits of one deployment. The server makes them
// configurable and reports them through GET /v1/info.
type Limits struct {
	MaxFiles     int
	MaxFileSize  int64
	MaxTotalSize int64
}

// DefaultLimits returns the default limits from ADR 0004.
func DefaultLimits() Limits {
	return Limits{
		MaxFiles:     10_000,
		MaxFileSize:  50 << 20,
		MaxTotalSize: 500 << 20,
	}
}

// CheckLimits reports the first limit that manifest goes over, naming it in
// the error. It expects a manifest that Decode accepted.
func (manifest *Manifest) CheckLimits(limits Limits) error {
	if len(manifest.Files) > limits.MaxFiles {
		return fmt.Errorf("%w: %d files exceeds the limit of %d files per deployment", ErrLimitExceeded, len(manifest.Files), limits.MaxFiles)
	}
	var totalSize int64
	for index, file := range manifest.Files {
		if file.Size > limits.MaxFileSize {
			return fmt.Errorf("%w: files[%d] is %d bytes, over the limit of %d bytes for the size of one file", ErrLimitExceeded, index, file.Size, limits.MaxFileSize)
		}
		// Compare without adding, so a large limit cannot overflow the total.
		if file.Size > limits.MaxTotalSize-totalSize {
			return fmt.Errorf("%w: files[%d] takes the total size over the limit of %d bytes", ErrLimitExceeded, index, limits.MaxTotalSize)
		}
		totalSize += file.Size
	}
	return nil
}
