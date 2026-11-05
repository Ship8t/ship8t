package storage

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ship8t/ship8t/internal/deployment"
	"github.com/ship8t/ship8t/internal/manifest"
)

// expiryNowTest is the time of the sweeps in these tests, a whole second so that
// the tests can compute update times from it.
var expiryNowTest = time.Unix(1_000_000, 0)

// idleIDTest returns a deployment ID of the shape that the table accepts, made
// from the number, so IDs sort in the order of their numbers.
func idleIDTest(number int) string {
	return fmt.Sprintf("%032x", number)
}

// insertIdleTest adds a deployment of the project in the state with the given
// update time.
func insertIdleTest(t *testing.T, database *Database, number int, projectID, state string, updatedAt int64) {
	t.Helper()

	_, err := database.connection.ExecContext(t.Context(),
		"INSERT INTO deployments (id, project_id, state, manifest, updated_at) VALUES (?, ?, ?, ?, ?)",
		idleIDTest(number), projectID, state, `{"kept":true}`, updatedAt)
	if err != nil {
		t.Fatalf("insert deployment %d in state %q: %v", number, state, err)
	}
}

// updatedAtAgo returns the update time of a deployment that has been idle for
// the age at expiryNowTest.
func updatedAtAgo(age time.Duration) int64 {
	return expiryNowTest.Unix() - int64(age/time.Second)
}

func TestExpireIdleDeploymentsExpiresFromTheTimeoutOnward(t *testing.T) {
	// Each timeout is tried one second inside, at, and one second past its edge,
	// and two timeouts show that the code does not hardcode one hour.
	for _, timeout := range []time.Duration{time.Hour, 90 * time.Second} {
		t.Run(timeout.String(), func(t *testing.T) {
			database := ensureTestSchema(t)
			insertIdleTest(t, database, 1, "my-site", "created", updatedAtAgo(timeout-time.Second))
			insertIdleTest(t, database, 2, "my-site", "created", updatedAtAgo(timeout))
			insertIdleTest(t, database, 3, "my-site", "created", updatedAtAgo(timeout+time.Second))

			got, err := database.expireIdleDeploymentsAt(t.Context(), expiryNowTest, timeout)

			if err != nil {
				t.Fatalf("expireIdleDeploymentsAt() error = %v, want nil", err)
			}
			if want := []string{idleIDTest(2), idleIDTest(3)}; !slices.Equal(got, want) {
				t.Errorf("expired IDs = %v, want %v", got, want)
			}
			rows := deploymentRows(t, database)
			if len(rows) != 3 {
				t.Fatalf("deployments = %d, want 3", len(rows))
			}
			if rows[0].state != "created" || rows[0].updatedAt != updatedAtAgo(timeout-time.Second) {
				t.Errorf("deployment idle for one second less = state %q, updated_at %d, want it unchanged", rows[0].state, rows[0].updatedAt)
			}
			for _, row := range rows[1:] {
				if row.state != "expired" || row.updatedAt != expiryNowTest.Unix() {
					t.Errorf("deployment %s = state %q, updated_at %d, want %q and %d", row.id, row.state, row.updatedAt, "expired", expiryNowTest.Unix())
				}
				if row.projectID != "my-site" || row.manifest != `{"kept":true}` {
					t.Errorf("deployment %s project and manifest = %q and %q, want them unchanged", row.id, row.projectID, row.manifest)
				}
			}
		})
	}
}

func TestExpireIdleDeploymentsFollowsTheTransitionTable(t *testing.T) {
	// The states are written out, so a new state needs a decision here. The test
	// also asks Transition, so the list of states that the sweep selects cannot
	// drift from the state machine.
	tests := []struct {
		state       deployment.State
		wantExpired bool
	}{
		{state: deployment.Created, wantExpired: true},
		{state: deployment.Uploading, wantExpired: true},
		{state: deployment.Ready, wantExpired: false},
		{state: deployment.Failed, wantExpired: false},
		{state: deployment.Cancelled, wantExpired: false},
		{state: deployment.Expired, wantExpired: false},
		{state: deployment.Pruned, wantExpired: false},
	}
	for _, test := range tests {
		t.Run(string(test.state), func(t *testing.T) {
			changed, _ := deployment.Transition(test.state, deployment.Expired)
			if changed != test.wantExpired {
				t.Fatalf("Transition(%q, expired) changed = %v, want %v", test.state, changed, test.wantExpired)
			}
			database := ensureTestSchema(t)
			idleSince := updatedAtAgo(48 * time.Hour)
			insertIdleTest(t, database, 1, "my-site", string(test.state), idleSince)

			got, err := database.expireIdleDeploymentsAt(t.Context(), expiryNowTest, time.Hour)

			if err != nil {
				t.Fatalf("expireIdleDeploymentsAt() error = %v, want nil", err)
			}
			rows := deploymentRows(t, database)
			if len(rows) != 1 {
				t.Fatalf("deployments = %d, want 1", len(rows))
			}
			if test.wantExpired {
				if !slices.Equal(got, []string{idleIDTest(1)}) || rows[0].state != "expired" || rows[0].updatedAt != expiryNowTest.Unix() {
					t.Errorf("IDs = %v, row = state %q, updated_at %d, want one expired deployment at %d", got, rows[0].state, rows[0].updatedAt, expiryNowTest.Unix())
				}
				return
			}
			if len(got) != 0 || rows[0].state != string(test.state) || rows[0].updatedAt != idleSince {
				t.Errorf("IDs = %v, row = state %q, updated_at %d, want no ID and the row unchanged", got, rows[0].state, rows[0].updatedAt)
			}
		})
	}
}

func TestExpireIdleDeploymentsRejectsATimeoutBelowOneSecond(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
		want    string
	}{
		{name: "zero", timeout: 0, want: "idle timeout is 0s and must be at least 1 second, so pass how long a deployment may stay idle"},
		{name: "negative", timeout: -time.Hour, want: "idle timeout is -1h0m0s and must be at least 1 second, so pass how long a deployment may stay idle"},
		{name: "under a second", timeout: 999 * time.Millisecond, want: "idle timeout is 999ms and must be at least 1 second, so pass how long a deployment may stay idle"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			database := ensureTestSchema(t)
			insertIdleTest(t, database, 1, "my-site", "created", updatedAtAgo(48*time.Hour))

			got, err := database.expireIdleDeploymentsAt(t.Context(), expiryNowTest, test.timeout)

			if err == nil || err.Error() != test.want {
				t.Errorf("expireIdleDeploymentsAt() error = %v, want %q", err, test.want)
			}
			if len(got) != 0 {
				t.Errorf("expired IDs = %v, want none", got)
			}
			rows := deploymentRows(t, database)
			if len(rows) != 1 || rows[0].state != "created" {
				t.Errorf("rows after the refusal = %+v, want the deployment unchanged", rows)
			}
		})
	}
}

func TestExpireIdleDeploymentsAcceptsATimeoutOfOneSecond(t *testing.T) {
	database := ensureTestSchema(t)
	insertIdleTest(t, database, 1, "my-site", "created", updatedAtAgo(time.Second))
	insertIdleTest(t, database, 2, "my-site", "created", expiryNowTest.Unix())

	got, err := database.expireIdleDeploymentsAt(t.Context(), expiryNowTest, time.Second)

	if err != nil {
		t.Fatalf("expireIdleDeploymentsAt() error = %v, want nil", err)
	}
	if want := []string{idleIDTest(1)}; !slices.Equal(got, want) {
		t.Errorf("expired IDs = %v, want %v", got, want)
	}
}

func TestExpireIdleDeploymentsReturnsTheIDsInOrder(t *testing.T) {
	database := ensureTestSchema(t)
	for _, number := range []int{3, 1, 2} {
		insertIdleTest(t, database, number, "my-site", "uploading", updatedAtAgo(2*time.Hour))
	}

	got, err := database.expireIdleDeploymentsAt(t.Context(), expiryNowTest, time.Hour)

	if err != nil {
		t.Fatalf("expireIdleDeploymentsAt() error = %v, want nil", err)
	}
	if want := []string{idleIDTest(1), idleIDTest(2), idleIDTest(3)}; !slices.Equal(got, want) {
		t.Errorf("expired IDs = %v, want %v", got, want)
	}
}

func TestExpireIdleDeploymentsExpiresNothingWhenNothingIsIdle(t *testing.T) {
	database := ensureTestSchema(t)
	insertIdleTest(t, database, 1, "my-site", "created", expiryNowTest.Unix())

	got, err := database.expireIdleDeploymentsAt(t.Context(), expiryNowTest, time.Hour)

	if err != nil {
		t.Fatalf("expireIdleDeploymentsAt() error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("expired IDs = %v, want none", got)
	}
}

func TestExpireIdleDeploymentsExpiresInEveryProject(t *testing.T) {
	database := ensureTestSchema(t)
	execTest(t, database, "INSERT INTO projects (id) VALUES ('other-site')")
	insertIdleTest(t, database, 1, "my-site", "created", updatedAtAgo(2*time.Hour))
	insertIdleTest(t, database, 2, "other-site", "uploading", updatedAtAgo(2*time.Hour))
	insertIdleTest(t, database, 3, "other-site", "uploading", updatedAtAgo(time.Minute))

	got, err := database.expireIdleDeploymentsAt(t.Context(), expiryNowTest, time.Hour)

	if err != nil {
		t.Fatalf("expireIdleDeploymentsAt() error = %v, want nil", err)
	}
	if want := []string{idleIDTest(1), idleIDTest(2)}; !slices.Equal(got, want) {
		t.Errorf("expired IDs = %v, want %v", got, want)
	}
	rows := deploymentRows(t, database)
	if rows[2].state != "uploading" {
		t.Errorf("recent deployment state = %q, want %q", rows[2].state, "uploading")
	}
}

func TestExpireIdleDeploymentsAcceptsARepeatAndChangesNothing(t *testing.T) {
	database := ensureTestSchema(t)
	insertIdleTest(t, database, 1, "my-site", "created", updatedAtAgo(2*time.Hour))
	if _, err := database.expireIdleDeploymentsAt(t.Context(), expiryNowTest, time.Hour); err != nil {
		t.Fatalf("first expireIdleDeploymentsAt() error = %v, want nil", err)
	}

	got, err := database.expireIdleDeploymentsAt(t.Context(), expiryNowTest.Add(time.Hour), time.Hour)

	if err != nil {
		t.Fatalf("second expireIdleDeploymentsAt() error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("expired IDs of the repeat = %v, want none", got)
	}
	rows := deploymentRows(t, database)
	if len(rows) != 1 || rows[0].state != "expired" || rows[0].updatedAt != expiryNowTest.Unix() {
		t.Errorf("rows after the repeat = %+v, want one expired row with updated_at %d", rows, expiryNowTest.Unix())
	}
}

func TestExpireIdleDeploymentsFreesRoomUnderTheUnfinishedLimit(t *testing.T) {
	database := ensureTestSchema(t)
	if _, err := database.CreateDeployment(t.Context(), "my-site", validManifestTest(), manifest.DefaultLimits(), 1); err != nil {
		t.Fatalf("CreateDeployment() error = %v, want nil", err)
	}
	if _, err := database.CreateDeployment(t.Context(), "my-site", validManifestTest(), manifest.DefaultLimits(), 1); err == nil {
		t.Fatal("CreateDeployment() at the limit error = nil, want an error")
	}

	got, err := database.expireIdleDeploymentsAt(t.Context(), time.Now().Add(2*time.Hour), time.Hour)

	if err != nil || len(got) != 1 {
		t.Fatalf("expireIdleDeploymentsAt() = %v, %v, want one ID and nil", got, err)
	}
	if _, err := database.CreateDeployment(t.Context(), "my-site", validManifestTest(), manifest.DefaultLimits(), 1); err != nil {
		t.Errorf("CreateDeployment() after an expiry error = %v, want nil", err)
	}
}

func TestExpireIdleDeploymentsUsesTheSystemClock(t *testing.T) {
	database := ensureTestSchema(t)
	now := time.Now().Unix()
	insertIdleTest(t, database, 1, "my-site", "created", now-7200)
	insertIdleTest(t, database, 2, "my-site", "created", now)

	before := time.Now().Unix()
	got, err := database.ExpireIdleDeployments(t.Context(), time.Hour)
	after := time.Now().Unix()

	if err != nil {
		t.Fatalf("ExpireIdleDeployments() error = %v, want nil", err)
	}
	if want := []string{idleIDTest(1)}; !slices.Equal(got, want) {
		t.Errorf("expired IDs = %v, want %v", got, want)
	}
	rows := deploymentRows(t, database)
	if rows[0].updatedAt < before || rows[0].updatedAt > after {
		t.Errorf("updated_at = %d, want between %d and %d", rows[0].updatedAt, before, after)
	}
	if rows[1].state != "created" {
		t.Errorf("recent deployment state = %q, want %q", rows[1].state, "created")
	}
}

func TestExpireIdleDeploymentsReturnsContextError(t *testing.T) {
	database := ensureTestSchema(t)
	insertIdleTest(t, database, 1, "my-site", "created", updatedAtAgo(2*time.Hour))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := database.expireIdleDeploymentsAt(ctx, expiryNowTest, time.Hour)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expireIdleDeploymentsAt() error = %v, want it to wrap %v", err, context.Canceled)
	}
	if err == nil || !strings.HasPrefix(err.Error(), "expire idle deployments: begin transaction: ") {
		t.Errorf("expireIdleDeploymentsAt() error = %v, want the prefix %q", err, "expire idle deployments: begin transaction: ")
	}
	rows := deploymentRows(t, database)
	if len(rows) != 1 || rows[0].state != "created" {
		t.Errorf("rows after the refusal = %+v, want the deployment unchanged", rows)
	}
}

func TestExpireIdleDeploymentsReportsAFailedSelect(t *testing.T) {
	database := ensureTestSchema(t)
	// A missing table makes the select fail, so no seam is needed to reach it.
	execTest(t, database, "DROP TABLE deployments")

	_, err := database.expireIdleDeploymentsAt(t.Context(), expiryNowTest, time.Hour)

	if err == nil || !strings.HasPrefix(err.Error(), "expire idle deployments: select candidates: ") {
		t.Errorf("expireIdleDeploymentsAt() error = %v, want the prefix %q", err, "expire idle deployments: select candidates: ")
	}
}

func TestExpireIdleDeploymentsReportsAFailedUpdateAndRollsBackTheSweep(t *testing.T) {
	database := ensureTestSchema(t)
	insertIdleTest(t, database, 1, "my-site", "created", updatedAtAgo(2*time.Hour))
	insertIdleTest(t, database, 2, "my-site", "uploading", updatedAtAgo(2*time.Hour))
	// A trigger fails the update of the second deployment only, so the first
	// update has run and the rollback must undo it. No seam is needed.
	execTest(t, database, fmt.Sprintf("CREATE TRIGGER refuse_update BEFORE UPDATE ON deployments WHEN OLD.id = '%s' BEGIN SELECT RAISE(ABORT, 'update refused'); END", idleIDTest(2)))

	got, err := database.expireIdleDeploymentsAt(t.Context(), expiryNowTest, time.Hour)

	if err == nil || !strings.HasPrefix(err.Error(), "expire idle deployments: update: ") || !strings.Contains(err.Error(), "update refused") {
		t.Errorf("expireIdleDeploymentsAt() error = %v, want the prefix %q and the cause", err, "expire idle deployments: update: ")
	}
	if len(got) != 0 {
		t.Errorf("expired IDs = %v, want none after a failure", got)
	}
	rows := deploymentRows(t, database)
	if len(rows) != 2 || rows[0].state != "created" || rows[1].state != "uploading" {
		t.Errorf("rows after the failure = %+v, want both deployments unchanged", rows)
	}

	// The failure must release the one connection, so a later call works.
	execTest(t, database, "DROP TRIGGER refuse_update")
	got, err = database.expireIdleDeploymentsAt(t.Context(), expiryNowTest, time.Hour)
	if err != nil || len(got) != 2 {
		t.Errorf("expireIdleDeploymentsAt() after the failure = %v, %v, want two IDs and nil", got, err)
	}
}
