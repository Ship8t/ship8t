package storage

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// layoutDirectoryNames lists the directories that sit in the data directory
// before any project exists, as ADR 0003 lays them out. The directories of a
// project are made with the project.
var layoutDirectoryNames = []string{"projects", "quarantine", "pruning"}

// CreateDataDirectoryLayout makes the directories of ADR 0003 that sit in the
// data directory with mode 0700, and leaves one that already exists alone. Call
// it after LockDataDirectory, so no other process changes the directory at the
// same time. It never creates the data directory.
func CreateDataDirectoryLayout(dataDirectory string) error {
	if err := checkDataDirectory(dataDirectory); err != nil {
		return err
	}

	root, err := os.OpenRoot(dataDirectory)
	if err != nil {
		return fmt.Errorf("create data directory layout: %w", err)
	}
	defer root.Close()

	// Check every existing entry before the first mkdir, so a refused layout
	// creates nothing.
	var missing []string
	for _, name := range layoutDirectoryNames {
		exists, err := checkLayoutEntry(root, dataDirectory, name)
		if err != nil {
			return err
		}
		if !exists {
			missing = append(missing, name)
		}
	}

	for _, name := range missing {
		if err := root.Mkdir(name, 0o700); err != nil {
			return fmt.Errorf("create data directory layout: %w", err)
		}
		// Set the mode apart from the mkdir, because the umask can clear bits of
		// a new directory and chmod is not subject to it.
		if err := root.Chmod(name, 0o700); err != nil {
			return fmt.Errorf("create data directory layout: %w", err)
		}
	}
	return nil
}

// checkLayoutEntry reports whether the entry exists, and rejects an entry that is
// not a directory or that group or other can use. It looks at the entry itself
// with Lstat, because os.Root follows a symlink that stays inside the directory,
// and a symlink to another volume would break the atomic rename between the
// directories of a project and pruning.
func checkLayoutEntry(root *os.Root, dataDirectory, name string) (bool, error) {
	info, err := root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create data directory layout: %w", err)
	}

	// Join as text and do not clean, so the path names the entry that the system
	// opens.
	path := dataDirectory + string(os.PathSeparator) + name
	if !info.IsDir() {
		return false, fmt.Errorf("%q is not a directory, so remove it and start again", path)
	}
	if err := checkDirectoryAccess(info, path, "data directory entry"); err != nil {
		return false, err
	}
	return true, nil
}
