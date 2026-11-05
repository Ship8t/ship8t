package storage

import (
	"fmt"
	"os"
)

// createProjectDirectories makes projects/<id>, with staging and releases inside
// it, in the data directory with mode 0700, and leaves one that already exists
// alone, so a repeat is safe. It applies the checks of the layout, and it needs
// projects/ to exist. It never creates the layout.
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

	exists, err := checkLayoutEntry(root, dataDirectory, "projects")
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%q does not exist, so create the data directory layout first", dataDirectory+string(os.PathSeparator)+"projects")
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
