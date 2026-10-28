// Package scan lists the files of a deploy directory for the client.
package scan

import (
	"fmt"
	"io/fs"
	"os"
	"slices"
)

// Options holds the rules from the project configuration that change what Scan
// lists.
type Options struct {
	// Ignore lists patterns of extra entries to skip, in the syntax of ADR 0001
	// section 7. They add to the default ignore rules and never replace them.
	Ignore []string
	// Include lists patterns of paths to add back after a default ignore rule
	// removed them. A pattern also adds back everything below a directory it
	// matches. It never overrides an Ignore pattern.
	Include []string
	// AllowSensitiveFiles turns off the check that fails the scan when a listed
	// file has a name that looks like a secret.
	AllowSensitiveFiles bool
}

// skipEntry reports whether the walk skips the entry at path, and the whole
// directory when the entry is one. An ignore pattern always skips. A default
// rule skips unless an include pattern adds the entry back. A directory that a
// default rule removed stays open when an include pattern could match below it.
func skipEntry(path string, isDir bool, ignorePatterns, includePatterns []pattern) bool {
	if matchesAny(ignorePatterns, path, isDir) {
		return true
	}
	if !removedByDefault(path, isDir) || matchesEntryOrParent(includePatterns, path, isDir) {
		return false
	}
	return !isDir || !mayMatchBelowAny(includePatterns, path)
}

// Scan returns the slash-separated paths of the regular files under directory,
// sorted by byte order. It skips the entries that the default ignore rules of
// ADR 0001 and the ignore patterns of options name, except the paths that the
// include patterns add back. It fails on any other symlink or file that is not
// a regular file, and on any listed file whose name looks like a secret, and
// names the path in the error.
func Scan(directory string, options Options) ([]string, error) {
	ignorePatterns, err := parsePatterns(options.Ignore)
	if err != nil {
		return nil, fmt.Errorf("ignore: %w", err)
	}
	includePatterns, err := parsePatterns(options.Include)
	if err != nil {
		return nil, fmt.Errorf("include: %w", err)
	}

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
		if path != "." && skipEntry(path, entry.IsDir(), ignorePatterns, includePatterns) {
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
	if !options.AllowSensitiveFiles {
		if err := checkSensitiveFiles(files); err != nil {
			return nil, err
		}
	}
	return files, nil
}
