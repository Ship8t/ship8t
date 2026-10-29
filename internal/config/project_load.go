package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// projectFileName is the name of the project config file in a project directory.
const projectFileName = "ship8t.json"

// LoadProjectFile reads the ship8t.json in directory and decodes it with
// DecodeProjectFile. A directory without the file gives the zero ProjectFile,
// because every setting is optional. It refuses a symlink and any other file
// that is not a regular file, so it never reads from somewhere else.
func LoadProjectFile(directory string) (ProjectFile, error) {
	// Read through os.Root, so the file cannot lead out of the project directory.
	root, err := os.OpenRoot(directory)
	if err != nil {
		return ProjectFile{}, fmt.Errorf("open project directory %q: %w", directory, withoutPath(err))
	}
	defer root.Close()

	// Check the type before the open, because opening a named pipe with no writer
	// blocks for ever.
	info, err := root.Lstat(projectFileName)
	if errors.Is(err, fs.ErrNotExist) {
		return ProjectFile{}, nil
	}
	if err != nil {
		return ProjectFile{}, fmt.Errorf("read %s in project directory %q: %w", projectFileName, directory, withoutPath(err))
	}
	if !info.Mode().IsRegular() {
		return ProjectFile{}, fmt.Errorf("%s in project directory %q is not a regular file, replace it with a regular file", projectFileName, directory)
	}

	opened, err := root.Open(projectFileName)
	if err != nil {
		return ProjectFile{}, fmt.Errorf("read %s in project directory %q: %w", projectFileName, directory, withoutPath(err))
	}
	defer opened.Close()

	file, err := DecodeProjectFile(opened)
	if err != nil {
		return ProjectFile{}, fmt.Errorf("project directory %q: %w", directory, err)
	}
	return file, nil
}

// withoutPath drops the raw path that a file system error repeats, so the
// caller can print the path once, quoted.
func withoutPath(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	return err
}
