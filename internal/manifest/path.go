// Package manifest validates deployment manifests for the client and the server.
package manifest

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	// maxPathBytes is the longest manifest path in bytes, from ADR 0004.
	maxPathBytes = 1024
	// maxSegmentBytes is the longest segment of a manifest path in bytes, from ADR 0004.
	maxSegmentBytes = 255
)

// reservedDeviceNames holds the Windows device names without a number suffix.
var reservedDeviceNames = map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true}

// ValidatePath reports why a manifest path is unsafe, or nil when it follows
// the path rules of ADR 0004.
func ValidatePath(path string) error {
	if path == "" {
		return errors.New("invalid path: path is empty")
	}
	// Check the length first so an error never echoes an oversized path.
	if len(path) > maxPathBytes {
		return fmt.Errorf("invalid path: %d bytes is longer than the %d byte limit", len(path), maxPathBytes)
	}
	if !utf8.ValidString(path) {
		return fmt.Errorf("invalid path %q: not valid utf-8", path)
	}
	// The JSON decoder replaces invalid bytes and lone surrogates with U+FFFD, so
	// a path that holds it may have been changed from what the client sent.
	if strings.ContainsRune(path, utf8.RuneError) {
		return fmt.Errorf("invalid path %q: contains U+FFFD, the replacement character that marks bytes that are not valid utf-8", path)
	}
	// Require NFC so one name has one spelling on every filesystem.
	if !norm.NFC.IsNormalString(path) {
		return fmt.Errorf("invalid path %q: not in Unicode NFC form, normalize the file name to NFC", path)
	}
	if strings.ContainsRune(path, '\\') {
		return fmt.Errorf("invalid path %q: use / as the separator, not a backslash", path)
	}
	if strings.ContainsFunc(path, unicode.IsControl) {
		return fmt.Errorf("invalid path %q: contains a control character", path)
	}
	for _, segment := range strings.Split(path, "/") {
		if err := validateSegment(segment); err != nil {
			return fmt.Errorf("invalid path %q: %w", path, err)
		}
	}
	return nil
}

// validateSegment reports why one path segment is unsafe, or nil when it is safe.
func validateSegment(segment string) error {
	switch {
	case segment == "":
		return errors.New("empty segment, remove the leading, trailing, or doubled /")
	case segment == "." || segment == "..":
		return fmt.Errorf("segment %q is not allowed", segment)
	case len(segment) > maxSegmentBytes:
		return fmt.Errorf("segment is longer than %d bytes", maxSegmentBytes)
	case strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, " "):
		return fmt.Errorf("segment %q ends in a dot or a space", segment)
	case isWindowsReservedName(segment):
		return fmt.Errorf("segment %q is a windows reserved name", segment)
	}
	return nil
}

// isWindowsReservedName reports whether a segment names a Windows device,
// with or without an extension, because "nul.txt" is reserved too.
func isWindowsReservedName(segment string) bool {
	stem, _, _ := strings.Cut(segment, ".")
	name := strings.ToUpper(stem)
	if reservedDeviceNames[name] {
		return true
	}
	return isNumberedDeviceName(name)
}

// isNumberedDeviceName reports whether an upper-case name is COM1 to COM9 or
// LPT1 to LPT9.
func isNumberedDeviceName(name string) bool {
	if len(name) != 4 {
		return false
	}
	prefix, digit := name[:3], name[3]
	return (prefix == "COM" || prefix == "LPT") && digit >= '1' && digit <= '9'
}
