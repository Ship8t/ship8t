package manifest

import (
	"fmt"
	"strings"
)

// checkPathCollisions reports two files that would clash on a case-insensitive
// filesystem, or a path that is both a file and the directory of another file.
// It expects paths that already passed ValidatePath, so each is short enough
// to quote.
func checkPathCollisions(files []File) error {
	indexByKey := make(map[string]int, len(files))
	for index, file := range files {
		key := strings.ToLower(file.Path)
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
		key := strings.ToLower(file.Path)
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
