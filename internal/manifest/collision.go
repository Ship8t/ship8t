package manifest

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// foldKey returns a key that is equal for paths that name one file on a
// filesystem that ignores letter case and Unicode normalization, such as APFS.
// It follows canonical caseless matching: normalize, fold the case, normalize.
func foldKey(path string) string {
	folded := cases.Fold().String(norm.NFD.String(path))
	// Also map every rune to its smallest simple fold variant, because the
	// x/text fold tables miss scripts such as Cherokee that APFS does fold.
	return norm.NFD.String(strings.Map(smallestFoldVariant, folded))
}

// smallestFoldVariant returns the smallest rune that is equal to r under
// simple case folding, so every case variant of a letter maps to one rune.
func smallestFoldVariant(r rune) rune {
	smallest := r
	for variant := unicode.SimpleFold(r); variant != r; variant = unicode.SimpleFold(variant) {
		smallest = min(smallest, variant)
	}
	return smallest
}

// checkPathCollisions reports two files that would clash on a case-insensitive
// filesystem, or a path that is both a file and the directory of another file.
// It expects paths that already passed ValidatePath, so each is short enough
// to quote.
func checkPathCollisions(files []File) error {
	keys := make([]string, len(files))
	indexByKey := make(map[string]int, len(files))
	for index, file := range files {
		key := foldKey(file.Path)
		keys[index] = key
		first, found := indexByKey[key]
		if !found {
			indexByKey[key] = index
			continue
		}
		if files[first].Path == file.Path {
			return fmt.Errorf("files[%d] %q is a duplicate of files[%d]", index, file.Path, first)
		}
		return fmt.Errorf("files[%d] %q differs only by case from files[%d] %q", index, file.Path, first, files[first].Path)
	}

	// Check directories in a second pass so the result does not depend on file order.
	for index, file := range files {
		key := keys[index]
		for end := 0; end < len(key); end++ {
			if key[end] != '/' {
				continue
			}
			if first, found := indexByKey[key[:end]]; found {
				return fmt.Errorf("files[%d] %q is a file, but files[%d] %q uses it as a directory", first, files[first].Path, index, file.Path)
			}
		}
	}
	return nil
}
