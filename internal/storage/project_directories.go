package storage

import (
	"fmt"
	"io/fs"
	"os"
)

// createProjectDirectories makes projects/<id>, with staging and releases inside
// it, in the data directory with mode 0700, and leaves one that already exists
// alone, so a repeat is safe. It applies the checks of the layout, and it needs
// projects/ and pruning/ to exist. It never creates the layout. It rejects a
// project whose existing directories are on another filesystem than pruning/,
// because ADR 0003 needs the atomic renames between them.
func createProjectDirectories(dataDirectory, id string) error {
	// Validate before any path is built from the ID.
	if err := validateProjectID(id); err != nil {
		return err
	}
	if err := checkDataDirectory(dataDirectory); err != nil {
		return err
	}

	root, err := os.OpenRoot(dataDirectory)
	if err != nil {
		return fmt.Errorf("create project directories: %w", err)
	}
	defer root.Close()

	// The project goes under projects/ and is moved into pruning/, so both must
	// exist before a project does.
	var paths [2]string
	var layout [2]fs.FileInfo
	for index, name := range []string{"projects", "pruning"} {
		exists, err := checkLayoutEntry(root, dataDirectory, name)
		if err != nil {
			return err
		}
		paths[index] = dataDirectory + string(os.PathSeparator) + name
		if !exists {
			return fmt.Errorf("%q does not exist, so create the data directory layout first", paths[index])
		}
		layout[index], err = root.Lstat(name)
		if err != nil {
			return fmt.Errorf("create project directories: %w", err)
		}
	}
	// A new directory is on the filesystem of its parent, so every directory is
	// on the one of pruning/ when projects/ and each existing entry below it are.
	if err := sameFilesystem(paths[0], paths[1], layout[0], layout[1]); err != nil {
		return err
	}

	// Check every existing entry before the first mkdir, so a refused project
	// creates nothing. A child of a missing parent is missing too.
	project := "projects/" + id
	var missing []string
	for _, name := range []string{project, project + "/staging", project + "/releases"} {
		exists, err := checkLayoutEntry(root, dataDirectory, name)
		if err != nil {
			return err
		}
		if !exists {
			missing = append(missing, name)
			continue
		}
		info, err := root.Lstat(name)
		if err != nil {
			return fmt.Errorf("create project directories: %w", err)
		}
		if err := sameFilesystem(dataDirectory+string(os.PathSeparator)+name, paths[1], info, layout[1]); err != nil {
			return err
		}
	}

	for _, name := range missing {
		if err := root.Mkdir(name, 0o700); err != nil {
			return fmt.Errorf("create project directories: %w", err)
		}
		// Set the mode apart from the mkdir, because the umask can clear bits of
		// a new directory, and the next mkdir needs this one to be writable.
		if err := root.Chmod(name, 0o700); err != nil {
			return fmt.Errorf("create project directories: %w", err)
		}
	}
	return nil
}
