package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ship8t/ship8t/internal/deployment"
)

// ExpireIdleDeployments moves every unfinished deployment that has seen no
// activity for idleTimeout to the expired state and returns their IDs in order,
// as ADR 0002 and ADR 0003 set out. It refuses a timeout below one second,
// because a timeout of zero or less would expire every unfinished deployment.
// It changes rows only. Removing staging/<id> is left to the code that makes
// the directory. Call it after EnsureSchema.
func (database *Database) ExpireIdleDeployments(ctx context.Context, idleTimeout time.Duration) ([]string, error) {
	return database.expireIdleDeploymentsAt(ctx, time.Now(), idleTimeout)
}

// expireIdleDeploymentsAt expires the deployments that were idle for idleTimeout
// or longer at now. It takes the time so that a test can place a deployment
// exactly on the edge of the timeout. A deployment expires when its update time
// is at or before now minus the timeout, in whole seconds. The read and the
// updates run in one plain transaction, and a failure rolls back the whole
// sweep.
func (database *Database) expireIdleDeploymentsAt(ctx context.Context, now time.Time, idleTimeout time.Duration) ([]string, error) {
	if idleTimeout < time.Second {
		return nil, fmt.Errorf("idle timeout is %s and must be at least 1 second, so pass how long a deployment may stay idle", idleTimeout)
	}

	transaction, err := database.connection.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("expire idle deployments: begin transaction: %w", err)
	}
	cutoff := now.Unix() - int64(idleTimeout/time.Second)
	expired, err := expireIdleDeploymentsIn(ctx, transaction, cutoff, now.Unix())
	if err != nil {
		return nil, errors.Join(err, transaction.Rollback())
	}
	if err := transaction.Commit(); err != nil {
		return nil, fmt.Errorf("expire idle deployments: commit: %w", err)
	}
	return expired, nil
}

// expireIdleDeploymentsIn selects the unfinished deployments that were last
// active at or before cutoff, asks the state machine for each move, and updates
// the rows, inside the transaction. The state filter only narrows the scan,
// because Transition decides which moves are allowed.
func expireIdleDeploymentsIn(ctx context.Context, transaction *sql.Tx, cutoff, updatedAt int64) ([]string, error) {
	type candidate struct {
		id    string
		state deployment.State
	}

	rows, err := transaction.QueryContext(ctx, "SELECT id, state FROM deployments WHERE state IN (?, ?) AND updated_at <= ? ORDER BY id",
		string(deployment.Created), string(deployment.Uploading), cutoff)
	if err != nil {
		return nil, fmt.Errorf("expire idle deployments: select candidates: %w", err)
	}
	// Read every candidate and close the cursor before the first update.
	var candidates []candidate
	for rows.Next() {
		var found candidate
		if err := rows.Scan(&found.id, &found.state); err != nil {
			return nil, errors.Join(fmt.Errorf("expire idle deployments: read candidate: %w", err), rows.Close())
		}
		candidates = append(candidates, found)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("expire idle deployments: read candidate: %w", err)
	}

	var expired []string
	for _, found := range candidates {
		changed, err := deployment.Transition(found.state, deployment.Expired)
		if err != nil {
			return nil, fmt.Errorf("expire idle deployments: %w", err)
		}
		if !changed {
			continue
		}
		if _, err := transaction.ExecContext(ctx, "UPDATE deployments SET state = ?, updated_at = ? WHERE id = ?", string(deployment.Expired), updatedAt, found.id); err != nil {
			return nil, fmt.Errorf("expire idle deployments: update: %w", err)
		}
		expired = append(expired, found.id)
	}
	return expired, nil
}
