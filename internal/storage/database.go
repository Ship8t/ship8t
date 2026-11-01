package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	// Register the pure Go SQLite driver, as ADR 0003 chooses.
	_ "modernc.org/sqlite"
)

// busyTimeoutMilliseconds is how long SQLite waits for a lock before it
// reports the database as busy.
const busyTimeoutMilliseconds = 5000

// databaseSettings lists the pragmas that ADR 0003 sets on the database. The
// value is what Open sets, and the expected value is what SQLite reports back,
// because SQLite spells some values differently when it reads them.
var databaseSettings = []struct {
	pragma        string
	value         string
	expectedValue string
}{
	{pragma: "busy_timeout", value: strconv.Itoa(busyTimeoutMilliseconds), expectedValue: strconv.Itoa(busyTimeoutMilliseconds)},
	{pragma: "journal_mode", value: "WAL", expectedValue: "wal"},
	{pragma: "synchronous", value: "FULL", expectedValue: "2"},
	{pragma: "foreign_keys", value: "ON", expectedValue: "1"},
}

// Database is the SQLite database that holds the state of the server. It uses
// one connection, so transactions run one at a time.
type Database struct {
	connection *sql.DB
}

// Open opens the SQLite database file at path, creates it when it is missing,
// and applies the settings of ADR 0003. It fails when SQLite does not accept
// a setting, so the server never runs with weaker durability than the ADR asks.
func Open(ctx context.Context, path string) (*Database, error) {
	if path == "" {
		return nil, errors.New("open database: path is empty")
	}
	// Reject a NUL byte, because SQLite cuts the name there and opens a
	// different file than the one named.
	if strings.ContainsRune(path, 0) {
		return nil, errors.New("open database: path contains a NUL byte")
	}

	// Resolve the path first, because a file URI with a relative path reads
	// its first segment as a host name.
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("open database: resolve path: %w", err)
	}

	if err := prepareDatabaseFiles(absolutePath); err != nil {
		return nil, err
	}

	connection, err := sql.Open("sqlite", dataSourceName(absolutePath))
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// Keep one connection so the single writer rule holds and every
	// transaction waits for the one before it.
	connection.SetMaxOpenConns(1)

	if err := verifySettings(ctx, connection); err != nil {
		return nil, errors.Join(err, connection.Close())
	}
	return &Database{connection: connection}, nil
}

// databaseFileSuffixes names the files that SQLite keeps for one database, as
// suffixes of the database file name. The empty suffix is the main file.
var databaseFileSuffixes = []string{"", "-wal", "-shm"}

// prepareDatabaseFiles rejects a database file that group or other can use and
// then makes the main file when it is missing, so the database files are never
// open to anyone but the owner. It checks all three files before it creates
// one, so a refused open creates nothing. It opens the parent directory with
// os.Root and reaches every file through that root.
func prepareDatabaseFiles(path string) error {
	directory, name := filepath.Split(path)
	root, err := os.OpenRoot(directory)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer root.Close()

	for _, suffix := range databaseFileSuffixes {
		if err := checkDatabaseFile(root, directory, name+suffix); err != nil {
			return err
		}
	}
	return createDatabaseFile(root, name)
}

// checkDatabaseFile accepts a missing entry or a regular file that only its
// owner can use, and rejects anything else. The -wal and -shm files can outlive
// a crash with the mode of an earlier run, and they hold the same pages as the
// main file. It looks at the entry itself with Lstat, because os.Root follows a
// symlink that stays inside the directory, and SQLite follows a symlink at the
// path, so a dangling one would make SQLite create its target.
func checkDatabaseFile(root *os.Root, directory, name string) error {
	info, err := root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	// Join as text, so the path names the entry that the system opens.
	path := directory + name
	if !info.Mode().IsRegular() {
		return fmt.Errorf("database file %q is not a regular file, so remove it and start again", path)
	}
	mode := info.Mode().Perm()
	if mode&groupOtherAccess == 0 {
		return nil
	}

	// Keep the message on one line, so a path cannot forge a second one.
	if command, ok := chmodCommand("600", path); ok {
		return fmt.Errorf("database file can be used by group or other, mode %04o, run: %s", mode, command)
	}
	return fmt.Errorf("database file %q can be used by group or other, mode %04o, run chmod 600 on that file", path, mode)
}

// createDatabaseFile makes an empty database file with mode 0600 when it is
// missing and leaves an existing entry alone. SQLite creates the file otherwise,
// with a mode that the umask leaves open to group and others, and it gives the
// -wal and -shm files the mode of this one. The file holds token hashes, so the
// P0 rule of docs/go-quality.md asks for 0600. It creates the file with O_EXCL,
// so a symlink at the path is not followed and nothing is created behind it.
func createDatabaseFile(root *os.Root, name string) error {
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open database: create file: %w", err)
	}
	// Set the mode apart from the create, because the umask can clear bits of a
	// new file and chmod is not subject to it.
	if err := file.Chmod(0o600); err != nil {
		return errors.Join(fmt.Errorf("open database: set file mode: %w", err), file.Close())
	}
	// Check Close, because a failed close of a written file can mean lost data.
	if err := file.Close(); err != nil {
		return fmt.Errorf("open database: create file: %w", err)
	}
	return nil
}

// Close closes the database. It waits for work in progress to finish.
func (database *Database) Close() error {
	if err := database.connection.Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
	}
	return nil
}

// dataSourceName returns the file URI of the database with every setting as a
// pragma, so each connection that the pool opens applies them again.
func dataSourceName(path string) string {
	query := url.Values{}
	for _, setting := range databaseSettings {
		query.Add("_pragma", fmt.Sprintf("%s(%s)", setting.pragma, setting.value))
	}
	source := url.URL{Scheme: "file", Path: path, RawQuery: query.Encode()}
	return source.String()
}

// verifySettings reads every setting back, because SQLite can leave a pragma
// unchanged without an error, such as WAL mode on a filesystem that cannot
// share memory.
func verifySettings(ctx context.Context, connection *sql.DB) error {
	for _, setting := range databaseSettings {
		// Build the statement from a constant name, because SQLite takes no
		// placeholder for a pragma name and no input reaches it.
		statement := "PRAGMA " + setting.pragma

		var value string
		if err := connection.QueryRowContext(ctx, statement).Scan(&value); err != nil {
			return fmt.Errorf("open database: read %s: %w", setting.pragma, err)
		}
		if value != setting.expectedValue {
			return fmt.Errorf("open database: %s is %q, want %q", setting.pragma, value, setting.expectedValue)
		}
	}
	return nil
}
