package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// schemaVersion is the value of user_version that marks a database made by this
// code. Version 1 grows with each table until the first release, and no
// migration exists.
const schemaVersion = 1

// schemaStatements creates the tables of schema version 1. The text is constant,
// because no input reaches it.
var schemaStatements = []string{
	// The slug rule is ADR 0003 for a project ID. The byte length term rejects a
	// NUL byte, because SQLite text functions stop at one and would let it pass.
	`CREATE TABLE projects (
		id TEXT NOT NULL PRIMARY KEY CHECK (
			length(CAST(id AS BLOB)) BETWEEN 1 AND 63
			AND length(CAST(id AS BLOB)) = length(id)
			AND id NOT GLOB '*[^a-z0-9-]*'
			AND id NOT GLOB '-*'
			AND id NOT GLOB '*-'
		)
	) STRICT`,
	// The states are the seven of ADR 0002, so a state read wrongly never
	// persists. RESTRICT keeps the history of a project that has deployments. No
	// rule limits the ID or the manifest size, because no ADR gives the shape of
	// an ID and Decode owns the manifest limit.
	`CREATE TABLE deployments (
		id TEXT NOT NULL PRIMARY KEY,
		project_id TEXT NOT NULL REFERENCES projects (id) ON DELETE RESTRICT,
		state TEXT NOT NULL CHECK (
			state IN ('created', 'uploading', 'ready', 'failed', 'cancelled', 'expired', 'pruned')
		),
		manifest TEXT NOT NULL
	) STRICT`,
}

// EnsureSchema makes the database ready for the server. It creates schema
// version 1 in an empty database, accepts a database that already has version 1,
// and refuses any other database without changing it. Call it after Open.
func (database *Database) EnsureSchema(ctx context.Context) error {
	return applySchema(ctx, database.connection, schemaStatements)
}

// applySchema runs the statements in one transaction when the database is empty.
// It takes the statements as a parameter only so that a test can make one of
// them fail, and EnsureSchema always passes schemaStatements.
func applySchema(ctx context.Context, connection *sql.DB, statements []string) error {
	transaction, err := connection.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ensure schema: begin transaction: %w", err)
	}
	if err := ensureSchemaIn(ctx, transaction, statements); err != nil {
		return errors.Join(err, transaction.Rollback())
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("ensure schema: commit: %w", err)
	}
	return nil
}

// ensureSchemaIn reads the version and the contents of the database and then
// creates the schema, all inside the transaction, so nothing can change between
// the reading and the creating.
func ensureSchemaIn(ctx context.Context, transaction *sql.Tx, statements []string) error {
	var version int
	if err := transaction.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("ensure schema: read user_version: %w", err)
	}
	if version == schemaVersion {
		return nil
	}
	if version != 0 {
		return fmt.Errorf("ensure schema: database has schema version %d, want version %d, so run the release that made it or use a new database file", version, schemaVersion)
	}

	// Skip the tables that SQLite keeps for itself, such as sqlite_sequence, which
	// a dropped AUTOINCREMENT table leaves behind. Escape the underscore, because
	// LIKE reads it as any character.
	var objects int
	err := transaction.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE name NOT LIKE 'sqlite\_%' ESCAPE '\'`).Scan(&objects)
	if err != nil {
		return fmt.Errorf("ensure schema: read schema objects: %w", err)
	}
	if objects != 0 {
		return fmt.Errorf("ensure schema: database has tables and schema version 0, want version %d, so use a new database file", schemaVersion)
	}

	// Set the version first, so a failed statement shows that the rollback undoes
	// it. SQLite takes no placeholder for a pragma, and the value is a constant.
	if _, err := transaction.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
		return fmt.Errorf("ensure schema: set user_version: %w", err)
	}
	for _, statement := range statements {
		if _, err := transaction.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("ensure schema: create tables: %w", err)
		}
	}
	return nil
}
