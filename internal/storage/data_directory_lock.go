package storage

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// lockFileName is the name of the lock file inside the data directory, as
// ADR 0003 lays it out.
const lockFileName = "lock"

// DataDirectoryLock is the exclusive lock on a data directory. Hold it for the
// life of the server, so two server processes never share the directory.
type DataDirectoryLock struct {
	file *os.File
}

// LockDataDirectory takes the exclusive lock on the data directory and fails at
// once when another process holds it. The operating system drops the lock when
// the process exits, so a crash leaves no stale lock behind.
func LockDataDirectory(dataDirectory string) (*DataDirectoryLock, error) {
	root, err := os.OpenRoot(dataDirectory)
	if err != nil {
		return nil, fmt.Errorf("lock data directory: %w", err)
	}
	defer root.Close()

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
