package manifest

import (
	"errors"
	"fmt"
)

// ErrLimitExceeded marks a manifest that goes over a deployment limit, so the
// server can answer with limit_exceeded.
var ErrLimitExceeded = errors.New("limit exceeded")

// MaxBodyBytes is the largest manifest body that Decode reads, from ADR 0004.
const MaxBodyBytes = 8 << 20

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

// errTooManyFiles reports a manifest with more files than the limit allows.
func errTooManyFiles(limit int) error {
	return fmt.Errorf("%w: more than the limit of %d files per deployment", ErrLimitExceeded, limit)
}

// checkLimits reports the first file size limit that manifest goes over,
// naming it in the error. Validate checks MaxFiles before it calls this.
func (manifest *Manifest) checkLimits(limits Limits) error {
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
