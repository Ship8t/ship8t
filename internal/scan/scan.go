// Package scan lists the files of a deploy directory for the client.
package scan

import (
	"fmt"
	"io/fs"
	"os"
	"slices"
)

// Scan returns the slash-separated paths of the regular files under directory,
// sorted by byte order. It skips the entries that the default ignore rules of
// ADR 0001 name. It fails on any other symlink or file that is not a regular
// file, and names the path in the error.
func Scan(directory string) ([]string, error) {
	// Read through os.Root, so no path can leave the deploy directory.
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, fmt.Errorf("open deploy directory: %w", err)
	}
	defer root.Close()

	files := []string{}
	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("scan %s: %w", path, walkErr)
		}
		// Skip ignored entries before the type checks, so an ignored symlink
		// or an ignored directory is never read.
		if path != "." && ignoredByDefault(entry.Name(), entry.IsDir()) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("scan %s: symlinks are not allowed, remove the symlink", path)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("scan %s: not a regular file, remove it from the deploy directory", path)
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Sort the full paths, because the walk visits directory "a" before file
	// "a.html", while a byte sort puts "a.html" first.
	slices.Sort(files)
	return files, nil
}
