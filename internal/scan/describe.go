package scan

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/ship8t/ship8t/internal/manifest"
)

// placeholderChecksum stands in for a checksum while Describe checks the paths
// and the sizes, before it reads any content.
var placeholderChecksum = strings.Repeat("0", 64)

// Describe returns a manifest entry with the size and the SHA-256 checksum for
// each of files, in the same order. The paths are slash-separated and relative
// to directory, as Scan returns them. It checks the result against the manifest
// rules and limits, so the server accepts the entries that it returns.
func Describe(directory string, files []string, limits manifest.Limits) ([]manifest.File, error) {
	// Read through os.Root, so no path can leave the deploy directory.
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, fmt.Errorf("open deploy directory: %w", err)
	}
	defer root.Close()

	entries := make([]manifest.File, 0, len(files))
	for _, file := range files {
		// Use Lstat, so a symlink that replaced a file after the scan is refused
		// and never followed.
		info, err := root.Lstat(file)
		if err != nil {
			return nil, fmt.Errorf("scan %q: %w", file, withoutPath(err))
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("scan %q: not a regular file, remove it from the deploy directory", file)
		}
		entries = append(entries, manifest.File{Path: file, Size: info.Size(), SHA256: placeholderChecksum})
	}

	// Check the paths and the sizes from the stat before reading, so a file over
	// a limit is never read.
	described := &manifest.Manifest{ManifestVersion: manifest.Version, Files: entries}
	if err := described.Validate(limits); err != nil {
		return nil, err
	}
	for index := range entries {
		size, checksum, err := hashFile(root, entries[index].Path, limits.MaxFileSize)
		if err != nil {
			return nil, err
		}
		entries[index].Size, entries[index].SHA256 = size, checksum
	}
	// Check again, because a file can change between the stat and the read.
	if err := described.Validate(limits); err != nil {
		return nil, err
	}
	return entries, nil
}

// hashFile reads file through root and returns the number of bytes it read and
// their SHA-256 checksum in lowercase hexadecimal. It reads one byte more than
// maxSize at most, so a file that grew past the limit shows as too large.
func hashFile(root *os.Root, file string, maxSize int64) (int64, string, error) {
	opened, err := root.Open(file)
	if err != nil {
		return 0, "", fmt.Errorf("scan %q: %w", file, withoutPath(err))
	}
	defer opened.Close()

	// A limit from a server can be the largest int64, which cannot take one more.
	readLimit := maxSize
	if readLimit < math.MaxInt64 {
		readLimit++
	}
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(opened, readLimit))
	if err != nil {
		return 0, "", fmt.Errorf("scan %q: %w", file, withoutPath(err))
	}
	return size, hex.EncodeToString(hash.Sum(nil)), nil
}
