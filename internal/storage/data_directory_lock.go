package storage

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// lockFileName is the name of the lock file inside the data directory, as
// ADR 0003 lays it out.
const lockFileName = "lock"

// groupOtherAccess holds the permission bits that let group or other use a file.
const groupOtherAccess fs.FileMode = 0o077

// DataDirectoryLock is the exclusive lock on a data directory. Hold it for the
// life of the server, so two server processes never share the directory.
type DataDirectoryLock struct {
	file *os.File
}

// LockDataDirectory takes the exclusive lock on the data directory and fails at
// once when another process holds it. The operating system drops the lock when
// the process exits, so a crash leaves no stale lock behind.
func LockDataDirectory(dataDirectory string) (*DataDirectoryLock, error) {
	if err := checkDataDirectory(dataDirectory); err != nil {
		return nil, err
	}

	root, err := os.OpenRoot(dataDirectory)
	if err != nil {
		return nil, fmt.Errorf("lock data directory: %w", err)
	}
	defer root.Close()

	if err := checkLockEntry(root, dataDirectory); err != nil {
		return nil, err
	}

	file, err := root.OpenFile(lockFileName, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("lock data directory: %w", err)
	}

	if err := lockFileExclusively(file); err != nil {
		closeErr := file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			err = fmt.Errorf("data directory %q is in use by another process, so stop that process or choose another data directory", dataDirectory)
		} else {
			err = fmt.Errorf("lock data directory: %w", err)
		}
		return nil, errors.Join(err, closeErr)
	}
	return &DataDirectoryLock{file: file}, nil
}

// Release gives up the lock, so another process can lock the data directory.
func (lock *DataDirectoryLock) Release() error {
	// Close the file, which releases the lock that it holds.
	if err := lock.file.Close(); err != nil {
		return fmt.Errorf("release data directory lock: %w", err)
	}
	return nil
}

// checkDataDirectory accepts an existing directory that only its owner can use.
// It rejects an empty path, a missing directory, a path that is not a directory,
// and a directory that group or other can use, in that order, each with an error
// that names the fix. It checks the path itself, because os.OpenRoot reports a
// file as a plain "not a directory" text that no code can match.
func checkDataDirectory(dataDirectory string) error {
	// Reject an empty path, because the system reports it as a missing file
	// and that would hide a caller that forgot to set the directory.
	if dataDirectory == "" {
		return errors.New("data directory path is empty")
	}

	info, err := os.Stat(dataDirectory)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("data directory %q does not exist, so create it before you start the server", dataDirectory)
	}
	// A path below a regular file fails with ENOTDIR instead of a result.
	if errors.Is(err, syscall.ENOTDIR) || (err == nil && !info.IsDir()) {
		return fmt.Errorf("data directory %q is not a directory, so point it at a directory", dataDirectory)
	}
	if err != nil {
		return fmt.Errorf("check data directory: %w", err)
	}
	return checkDirectoryAccess(info, dataDirectory, "data directory")
}

// checkDirectoryAccess rejects a directory that group or other can use. The data
// directory holds the database and the lock file, and a symlink put in place of
// the lock entry needs write access to it. The entries inside hold the projects,
// the quarantine, and the releases being pruned, so they get the same rule. It
// reads the mode of the directory that the path leads to, so a symlinked data
// directory is judged by its target. It does not check the opened directory
// again, because a swap between the check and the open needs write access to a
// parent of the path. The description starts the message, such as "data
// directory".
func checkDirectoryAccess(info fs.FileInfo, path, description string) error {
	mode := info.Mode().Perm()
	if mode&groupOtherAccess == 0 {
		return nil
	}

	// Keep the message on one line, so a path cannot forge a second one.
	if command, ok := chmodCommand("700", path); ok {
		return fmt.Errorf("%s can be used by group or other, mode %04o, run: %s", description, mode, command)
	}
	return fmt.Errorf("%s %q can be used by group or other, mode %04o, run chmod 700 on that directory", description, path, mode)
}

// checkLockEntry accepts a missing lock entry or a regular file and rejects
// anything else. It looks at the entry itself with Lstat, because os.Root
// follows a symlink that stays inside the directory, and opening a dangling
// symlink would create its target.
func checkLockEntry(root *os.Root, dataDirectory string) error {
	info, err := root.Lstat(lockFileName)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lock data directory: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("lock file %q is not a regular file, so remove it and start again", filepath.Join(dataDirectory, lockFileName))
	}
	return nil
}

// lockFileExclusively asks for an exclusive lock on the file without waiting.
func lockFileExclusively(file *os.File) error {
	rawFile, err := file.SyscallConn()
	if err != nil {
		return err
	}

	var lockErr error
	controlErr := rawFile.Control(func(descriptor uintptr) {
		lockErr = syscall.Flock(int(descriptor), syscall.LOCK_EX|syscall.LOCK_NB)
	})
	return errors.Join(controlErr, lockErr)
}
