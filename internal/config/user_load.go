package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"unicode"
)

// userFileName is the name of the user config file in the ship8t directory.
const userFileName = "config.json"

// groupOtherAccess is the part of a file mode that lets the group or other
// users read, write, or run a file.
const groupOtherAccess fs.FileMode = 0o077

// LoadUserFile reads config.json in directory, the ship8t directory of the user
// such as ~/.config/ship8t, and decodes it with DecodeUserFile. It gives the
// zero UserFile when the directory or the file does not exist, and it refuses a
// symlink, a file that is not regular, or a file that group or other can access
// before it reads a byte.
func LoadUserFile(directory string) (UserFile, error) {
	// Reject an empty path first, because OpenRoot reports it as a missing file and
	// the missing-directory rule below would hide a caller that forgot to resolve
	// the directory.
	if directory == "" {
		return UserFile{}, errors.New("user config directory is empty, pass the ship8t directory such as ~/.config/ship8t")
	}

	// Treat a missing directory as no settings, unlike a project directory,
	// because the user creates the ship8t directory and a new machine has none.
	// Report any other failure.
	root, err := os.OpenRoot(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return UserFile{}, nil
	}
	if err != nil {
		return UserFile{}, fmt.Errorf("open user config directory %q: %w", directory, withoutPath(err))
	}
	defer root.Close()

	// Check the type before the open, because opening a named pipe with no writer
	// blocks for ever.
	info, err := root.Lstat(userFileName)
	if errors.Is(err, fs.ErrNotExist) {
		return UserFile{}, nil
	}
	if err != nil {
		return UserFile{}, fmt.Errorf("read %s in user config directory %q: %w", userFileName, directory, withoutPath(err))
	}
	if err := checkUserFile(info, directory); err != nil {
		return UserFile{}, err
	}

	opened, err := root.Open(userFileName)
	if err != nil {
		return UserFile{}, fmt.Errorf("read %s in user config directory %q: %w", userFileName, directory, withoutPath(err))
	}
	defer opened.Close()

	// Check the file that was opened, because the path can change between the
	// check and the open.
	openedInfo, err := opened.Stat()
	if err != nil {
		return UserFile{}, fmt.Errorf("read %s in user config directory %q: %w", userFileName, directory, withoutPath(err))
	}
	if err := checkUserFile(openedInfo, directory); err != nil {
		return UserFile{}, err
	}

	file, err := DecodeUserFile(opened)
	if err != nil {
		return UserFile{}, fmt.Errorf("user config directory %q: %w", directory, err)
	}
	return file, nil
}

// checkUserFile reports why the config file in directory must not be read, which
// is a type other than a regular file or a mode that group or other can use.
func checkUserFile(info fs.FileInfo, directory string) error {
	// Name a symlink on its own, because the user may have linked the file on
	// purpose and needs to know that the loader refuses it by design.
	if info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("%s in user config directory %q is a symlink, and symlinks are refused on purpose so that the token is never read from another file, replace it with a regular file or link the whole ship8t directory instead", userFileName, directory)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s in user config directory %q is not a regular file, replace it with a regular file", userFileName, directory)
	}
	mode := info.Mode().Perm()
	if mode&groupOtherAccess == 0 {
		return nil
	}

	// Join as text and do not clean, because a ".." after a symlink leads
	// somewhere else than the cleaned path does.
	path := directory + string(os.PathSeparator) + userFileName

	// Keep the message on one line, so a path cannot forge a second one.
	if command, ok := chmodCommand(path); ok {
		return fmt.Errorf("user config can be used by group or other, mode %04o, run: %s", mode, command)
	}
	return fmt.Errorf("user config %q can be used by group or other, mode %04o, run chmod 600 on that file", path, mode)
}

// chmodCommand returns the command that limits path to its owner. It reports
// false when the path holds a character that is not safe to print as it is,
// such as a newline, an escape, or a direction change, because the caller would
// print a command that forges a line or hides its meaning.
func chmodCommand(path string) (string, bool) {
	for _, character := range path {
		if !unicode.IsPrint(character) || character == unicode.ReplacementChar {
			return "", false
		}
	}
	// Put -- before the mode, so a relative path that starts with a dash is not
	// read as an option. BSD chmod stops reading options at the mode.
	return "chmod -- 600 " + shellQuote(path), true
}

// shellQuote wraps text in single quotes for a POSIX shell. Inside single quotes
// every character is literal except the quote, which it closes, escapes, and
// opens again.
func shellQuote(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}
