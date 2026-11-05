package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// maxProjectIDLength is the longest project ID, the length of a DNS label.
const maxProjectIDLength = 63

// validateProjectID rejects an ID that is not a slug of ADR 0003, which is 1 to
// 63 bytes of lowercase letters, digits, and hyphens with no leading or trailing
// hyphen. It runs before any path is built from the ID. The table of projects
// holds the same rule as a backstop, and internal/config holds a copy for the
// client.
func validateProjectID(id string) error {
	// Check the length first, so an error never echoes an oversized value.
	if len(id) < 1 || len(id) > maxProjectIDLength {
		return fmt.Errorf("project ID must be 1 to %d characters, got %d", maxProjectIDLength, len(id))
	}
	for index := 0; index < len(id); index++ {
		character := id[index]
		isLowercaseLetter := character >= 'a' && character <= 'z'
		isDigit := character >= '0' && character <= '9'
		if !isLowercaseLetter && !isDigit && character != '-' {
			return fmt.Errorf("project ID %q must use only lowercase letters, digits, and hyphens", id)
		}
	}
	if id[0] == '-' || id[len(id)-1] == '-' {
		return fmt.Errorf("project ID %q must not start or end with a hyphen", id)
	}
	return nil
}

// CreateProject adds the project to the database and makes its directories in
// the data directory, in one transaction that commits last. It accepts a project
// that already exists, and it makes any directory that is missing, so a repeat is
// safe. A failure leaves no row. It can leave the empty directories of the
// project, which the next call adopts. Call it after EnsureSchema and
// CreateDataDirectoryLayout.
func (database *Database) CreateProject(ctx context.Context, dataDirectory, id string) error {
	transaction, err := database.connection.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("create project: begin transaction: %w", err)
	}
	if err := createProjectIn(ctx, transaction, dataDirectory, id); err != nil {
		return errors.Join(err, transaction.Rollback())
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("create project: commit: %w", err)
	}
	return nil
}

// createProjectIn makes the directories and then inserts the row, inside the
// transaction. The directories come first, because they check the ID and report
// a refusal in words, and the table would only report a failed CHECK.
func createProjectIn(ctx context.Context, transaction *sql.Tx, dataDirectory, id string) error {
	if err := createProjectDirectories(dataDirectory, id); err != nil {
		return err
	}
	if _, err := transaction.ExecContext(ctx, "INSERT INTO projects (id) VALUES (?) ON CONFLICT (id) DO NOTHING", id); err != nil {
		return fmt.Errorf("create project: insert: %w", err)
	}
	return nil
}
