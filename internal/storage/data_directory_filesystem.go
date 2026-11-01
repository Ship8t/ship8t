package storage

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// CheckLayoutFilesystem rejects a data directory whose projects/ and pruning/
// are on different filesystems, because ADR 0003 moves a release from one to
// the other with an atomic rename and a rename across filesystems fails. It
// applies the checks of the layout, so call it after CreateDataDirectoryLayout.
// It never creates or changes anything.
func CheckLayoutFilesystem(dataDirectory string) error {
	if err := checkDataDirectory(dataDirectory); err != nil {
		return err
	}

	root, err := os.OpenRoot(dataDirectory)
	if err != nil {
		return fmt.Errorf("check layout filesystem: %w", err)
	}
	defer root.Close()

	var paths [2]string
	var infos [2]fs.FileInfo
	for index, name := range []string{"projects", "pruning"} {
		exists, err := checkLayoutEntry(root, dataDirectory, name)
		if err != nil {
			return err
		}
		// Join as text and do not clean, as checkLayoutEntry does.
		paths[index] = dataDirectory + string(os.PathSeparator) + name
		if !exists {
			return fmt.Errorf("%q does not exist, so create the data directory layout first", paths[index])
		}
		// Look at the entry itself, so a symlink never stands for its target.
		infos[index], err = root.Lstat(name)
		if err != nil {
			return fmt.Errorf("check layout filesystem: %w", err)
		}
	}

	return sameFilesystem(paths[0], paths[1], infos[0], infos[1])
}

// sameFilesystem rejects two directories that are on different devices. It
// reads the device from the stat result of each and fails when one has none, so
// an unknown device never counts as the same. The paths only name the directories
// in the error, and %q keeps an unprintable path on one line. The error never
// holds a device ID.
func sameFilesystem(projectsPath, pruningPath string, projects, pruning fs.FileInfo) error {
	projectsStat, ok := projects.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("read the device of %q, got %T", projectsPath, projects.Sys())
	}
	pruningStat, ok := pruning.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("read the device of %q, got %T", pruningPath, pruning.Sys())
	}
	if projectsStat.Dev != pruningStat.Dev {
		return fmt.Errorf("%q and %q are on different filesystems, so put them on one filesystem", projectsPath, pruningPath)
	}
	return nil
}
