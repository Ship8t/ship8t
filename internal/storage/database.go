package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
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
