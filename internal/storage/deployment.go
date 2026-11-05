package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ship8t/ship8t/internal/deployment"
	"github.com/ship8t/ship8t/internal/manifest"
)

// CreateDeployment adds a deployment in the created state to the project and
// returns its ID. It validates the manifest again against limits, because the
// server never trusts the caller, and stores the canonical JSON of the manifest
// that it validated. The manifest comes from manifest.Decode or is built by code
// that validates it, and it must not be nil. It refuses a project that does not
// exist and a project that already has maxUnfinished deployments in the created
// or uploading state, so a limit of zero or less refuses every call. It makes no
// directory, because the first upload makes staging/<id>. The check, the count,
// and the insert run in one plain transaction. Call it after EnsureSchema and
// CreateProject.
func (database *Database) CreateDeployment(ctx context.Context, projectID string, deploymentManifest *manifest.Manifest, limits manifest.Limits, maxUnfinished int) (string, error) {
	if err := validateProjectID(projectID); err != nil {
		return "", err
	}
	if err := deploymentManifest.Validate(limits); err != nil {
		return "", fmt.Errorf("create deployment: %w", err)
	}
	canonical, err := json.Marshal(deploymentManifest)
	if err != nil {
		return "", fmt.Errorf("create deployment: encode manifest: %w", err)
	}
	now := time.Now()
	id, err := newDeploymentID(now)
	if err != nil {
		return "", fmt.Errorf("create deployment: %w", err)
	}

	transaction, err := database.connection.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("create deployment: begin transaction: %w", err)
	}
	if err := createDeploymentIn(ctx, transaction, projectID, id, string(canonical), now.Unix(), maxUnfinished); err != nil {
		return "", errors.Join(err, transaction.Rollback())
	}
	if err := transaction.Commit(); err != nil {
		return "", fmt.Errorf("create deployment: commit: %w", err)
	}
	return id, nil
}

// createDeploymentIn checks that the project exists and has room for another
// unfinished deployment, and then inserts the row, inside the transaction.
func createDeploymentIn(ctx context.Context, transaction *sql.Tx, projectID, id, manifestText string, updatedAt int64, maxUnfinished int) error {
	var exists bool
	if err := transaction.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM projects WHERE id = ?)", projectID).Scan(&exists); err != nil {
		return fmt.Errorf("create deployment: look up project: %w", err)
	}
	if !exists {
		return fmt.Errorf("project %q does not exist, so create it first", projectID)
	}

	var unfinished int
	err := transaction.QueryRowContext(ctx, "SELECT count(*) FROM deployments WHERE project_id = ? AND state IN (?, ?)",
		projectID, string(deployment.Created), string(deployment.Uploading)).Scan(&unfinished)
	if err != nil {
		return fmt.Errorf("create deployment: count unfinished deployments: %w", err)
	}
	if unfinished >= maxUnfinished {
		return fmt.Errorf("project %q has %d unfinished deployments and the limit is %d, so finish or cancel one first", projectID, unfinished, maxUnfinished)
	}

	_, err = transaction.ExecContext(ctx, "INSERT INTO deployments (id, project_id, state, manifest, updated_at) VALUES (?, ?, ?, ?, ?)",
		id, projectID, string(deployment.Created), manifestText, updatedAt)
	if err != nil {
		return fmt.Errorf("create deployment: insert: %w", err)
	}
	return nil
}

// CancelDeployment moves an unfinished deployment to the cancelled state and
// records the time of the change. It accepts a deployment that is already
// cancelled and leaves its row alone, so a repeated request is safe. It refuses
// any other state with an error that wraps deployment.ErrStateConflict, as ADR
// 0002 sets out. It changes the row only, because recovery deletes the staging
// directory of a terminal deployment. The ID must have the shape of
// newDeploymentID, and the check runs before any query. The read and the update
// run in one plain transaction.
func (database *Database) CancelDeployment(ctx context.Context, deploymentID string) error {
	if err := validateDeploymentID(deploymentID); err != nil {
		return err
	}

	transaction, err := database.connection.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cancel deployment: begin transaction: %w", err)
	}
	if err := cancelDeploymentIn(ctx, transaction, deploymentID, time.Now().Unix()); err != nil {
		return errors.Join(err, transaction.Rollback())
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("cancel deployment: commit: %w", err)
	}
	return nil
}

// cancelDeploymentIn reads the state of the deployment, checks the move with the
// state machine, and updates the row only when the state changes, inside the
// transaction.
func cancelDeploymentIn(ctx context.Context, transaction *sql.Tx, deploymentID string, updatedAt int64) error {
	var state string
	err := transaction.QueryRowContext(ctx, "SELECT state FROM deployments WHERE id = ?", deploymentID).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("deployment %q does not exist", deploymentID)
	}
	if err != nil {
		return fmt.Errorf("cancel deployment: look up deployment: %w", err)
	}

	changed, err := deployment.Transition(deployment.State(state), deployment.Cancelled)
	if err != nil {
		return fmt.Errorf("cancel deployment: %w", err)
	}
	if !changed {
		return nil
	}

	if _, err := transaction.ExecContext(ctx, "UPDATE deployments SET state = ?, updated_at = ? WHERE id = ?", string(deployment.Cancelled), updatedAt, deploymentID); err != nil {
		return fmt.Errorf("cancel deployment: update: %w", err)
	}
	return nil
}
