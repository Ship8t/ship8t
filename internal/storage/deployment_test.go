package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ship8t/ship8t/internal/deployment"
	"github.com/ship8t/ship8t/internal/manifest"
)

// deploymentRow is one row of the deployments table, as the tests read it.
type deploymentRow struct {
	id        string
	projectID string
	state     string
	manifest  string
	updatedAt int64
}

// deploymentRows returns every row of the deployments table, ordered by ID.
func deploymentRows(t *testing.T, database *Database) []deploymentRow {
	t.Helper()

	rows, err := database.connection.QueryContext(t.Context(), "SELECT id, project_id, state, manifest, updated_at FROM deployments ORDER BY id")
	if err != nil {
		t.Fatalf("query deployments: %v", err)
	}
	defer rows.Close()
	var result []deploymentRow
	for rows.Next() {
		var row deploymentRow
		if err := rows.Scan(&row.id, &row.projectID, &row.state, &row.manifest, &row.updatedAt); err != nil {
			t.Fatalf("scan deployment: %v", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read deployments: %v", err)
	}
	return result
}

// validManifestTest returns a manifest with two files that passes the default
// limits.
func validManifestTest() *manifest.Manifest {
	return &manifest.Manifest{
		ManifestVersion: manifest.Version,
		Files: []manifest.File{
			{Path: "index.html", Size: 12, SHA256: strings.Repeat("a", 64)},
			{Path: "assets/app.js", Size: 34, SHA256: strings.Repeat("b", 64)},
		},
	}
}

// insertUnfinishedTest adds a deployment of my-site in the given state, with an
// ID made from the number.
func insertUnfinishedTest(t *testing.T, database *Database, projectID string, number int, state string) {
	t.Helper()

	id := fmt.Sprintf("%032x", number)
	if err := insertDeploymentTest(t.Context(), database, id, projectID, state, "{}"); err != nil {
		t.Fatalf("insert deployment %d in state %q: %v", number, state, err)
	}
}

func TestCreateDeploymentStoresACreatedDeployment(t *testing.T) {
	database := ensureTestSchema(t)
	input := validManifestTest()

	before := time.Now()
	id, err := database.CreateDeployment(t.Context(), "my-site", input, manifest.DefaultLimits(), 10)
	after := time.Now()
	if err != nil {
		t.Fatalf("CreateDeployment() error = %v, want nil", err)
	}

	rows := deploymentRows(t, database)
	if len(rows) != 1 {
		t.Fatalf("deployments = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.id != id {
		t.Errorf("stored ID = %q, want the returned ID %q", row.id, id)
	}
	if row.projectID != "my-site" {
		t.Errorf("project_id = %q, want %q", row.projectID, "my-site")
	}
	if row.state != "created" {
		t.Errorf("state = %q, want %q", row.state, "created")
	}
	if row.updatedAt < before.Unix() || row.updatedAt > after.Unix() {
		t.Errorf("updated_at = %d, want between %d and %d", row.updatedAt, before.Unix(), after.Unix())
	}
	if len(id) != 32 {
		t.Fatalf("ID = %q, want 32 characters", id)
	}
	milliseconds, err := strconv.ParseInt(id[:12], 16, 64)
	if err != nil {
		t.Fatalf("parse the time of ID %q: %v", id, err)
	}
	if milliseconds < before.UnixMilli() || milliseconds > after.UnixMilli() {
		t.Errorf("time of ID %q = %d, want between %d and %d", id, milliseconds, before.UnixMilli(), after.UnixMilli())
	}
}

func TestCreateDeploymentStoresTheCanonicalManifest(t *testing.T) {
	database := ensureTestSchema(t)
	input := validManifestTest()
	input.Files[0].Path = "a<b>&c.html"
	input.Serve.SPAFallback = true

	if _, err := database.CreateDeployment(t.Context(), "my-site", input, manifest.DefaultLimits(), 10); err != nil {
		t.Fatalf("CreateDeployment() error = %v, want nil", err)
	}

	want, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal the manifest: %v", err)
	}
	rows := deploymentRows(t, database)
	if len(rows) != 1 {
		t.Fatalf("deployments = %d, want 1", len(rows))
	}
	stored := rows[0].manifest
	if stored != string(want) {
		t.Errorf("stored manifest = %s, want %s", stored, want)
	}
	// Expect the server to read the stored text back as the manifest it was given.
	decoded, err := manifest.Decode(strings.NewReader(stored), manifest.DefaultLimits())
	if err != nil {
		t.Fatalf("Decode(stored manifest) error = %v, want nil", err)
	}
	if decoded.Files[0] != input.Files[0] || decoded.Files[1] != input.Files[1] || decoded.Serve != input.Serve {
		t.Errorf("decoded manifest = %+v, want %+v", decoded, input)
	}
}

func TestCreateDeploymentReturnsADistinctIDEachTime(t *testing.T) {
	database := ensureTestSchema(t)

	seen := map[string]bool{}
	for range 5 {
		id, err := database.CreateDeployment(t.Context(), "my-site", validManifestTest(), manifest.DefaultLimits(), 10)
		if err != nil {
			t.Fatalf("CreateDeployment() error = %v, want nil", err)
		}
		seen[id] = true
	}

	if len(seen) != 5 {
		t.Errorf("distinct IDs = %d, want 5", len(seen))
	}
	if got := len(deploymentRows(t, database)); got != 5 {
		t.Errorf("deployments = %d, want 5", got)
	}
}

func TestCreateDeploymentRejectsInvalidProjectID(t *testing.T) {
	ids := []string{"", "..", "../site", "a/b", "a\x00b", "Site", "-site", strings.Repeat("a", 64)}

	for _, id := range ids {
		t.Run(fmt.Sprintf("%q", id), func(t *testing.T) {
			database := ensureTestSchema(t)

			created, err := database.CreateDeployment(t.Context(), id, validManifestTest(), manifest.DefaultLimits(), 10)

			want := validateProjectID(id)
			if err == nil || err.Error() != want.Error() {
				t.Errorf("CreateDeployment(%q) error = %v, want %v", id, err, want)
			}
			if created != "" {
				t.Errorf("CreateDeployment(%q) = %q, want no ID with an error", id, created)
			}
			if got := len(deploymentRows(t, database)); got != 0 {
				t.Errorf("deployments after the refusal = %d, want 0", got)
			}
		})
	}
}

func TestCreateDeploymentRejectsAMissingProject(t *testing.T) {
	database := ensureTestSchema(t)

	id, err := database.CreateDeployment(t.Context(), "other-site", validManifestTest(), manifest.DefaultLimits(), 10)

	want := `project "other-site" does not exist, so create it first`
	if err == nil || err.Error() != want {
		t.Errorf("CreateDeployment() error = %v, want %q", err, want)
	}
	if id != "" {
		t.Errorf("CreateDeployment() = %q, want no ID with an error", id)
	}
	if got := len(deploymentRows(t, database)); got != 0 {
		t.Errorf("deployments after the refusal = %d, want 0", got)
	}
}

func TestCreateDeploymentRejectsAnInvalidManifest(t *testing.T) {
	tooBig := validManifestTest()
	tooBig.Files[0].Size = 60

	tests := []struct {
		name     string
		input    *manifest.Manifest
		limits   manifest.Limits
		wantIs   error
		wantText string
	}{
		{name: "zero manifest", input: &manifest.Manifest{}, limits: manifest.DefaultLimits(), wantText: "unsupported manifest version"},
		{name: "unsupported version", input: &manifest.Manifest{ManifestVersion: 2, Files: validManifestTest().Files}, limits: manifest.DefaultLimits(), wantIs: manifest.ErrUnsupportedVersion},
		{name: "no files", input: &manifest.Manifest{ManifestVersion: 1}, limits: manifest.DefaultLimits(), wantText: "files must list at least one file"},
		{name: "path leaves the directory", input: &manifest.Manifest{ManifestVersion: 1, Files: []manifest.File{{Path: "../x", Size: 1, SHA256: strings.Repeat("a", 64)}}}, limits: manifest.DefaultLimits(), wantText: "invalid path"},
		{name: "bad checksum", input: &manifest.Manifest{ManifestVersion: 1, Files: []manifest.File{{Path: "x", Size: 1, SHA256: "ABC"}}}, limits: manifest.DefaultLimits(), wantText: "invalid sha256"},
		{name: "paths that differ by case", input: &manifest.Manifest{ManifestVersion: 1, Files: []manifest.File{{Path: "a.html", Size: 1, SHA256: strings.Repeat("a", 64)}, {Path: "A.html", Size: 1, SHA256: strings.Repeat("a", 64)}}}, limits: manifest.DefaultLimits(), wantText: "differs only by case"},
		// The next three pass the default limits and fail the limits that the
		// caller passes, so the code must use the caller's.
		{name: "more files than the limit", input: validManifestTest(), limits: manifest.Limits{MaxFiles: 1, MaxFileSize: 50, MaxTotalSize: 100}, wantIs: manifest.ErrLimitExceeded},
		{name: "file over the size limit", input: tooBig, limits: manifest.Limits{MaxFiles: 10, MaxFileSize: 50, MaxTotalSize: 100}, wantIs: manifest.ErrLimitExceeded},
		{name: "total over the size limit", input: validManifestTest(), limits: manifest.Limits{MaxFiles: 10, MaxFileSize: 50, MaxTotalSize: 40}, wantIs: manifest.ErrLimitExceeded},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			database := ensureTestSchema(t)

			id, err := database.CreateDeployment(t.Context(), "my-site", test.input, test.limits, 10)

			if err == nil {
				t.Fatalf("CreateDeployment() error = nil, want an error")
			}
			if test.wantIs != nil && !errors.Is(err, test.wantIs) {
				t.Errorf("CreateDeployment() error = %v, want it to wrap %v", err, test.wantIs)
			}
			if test.wantText != "" && !strings.Contains(err.Error(), test.wantText) {
				t.Errorf("CreateDeployment() error = %v, want it to contain %q", err, test.wantText)
			}
			if id != "" {
				t.Errorf("CreateDeployment() = %q, want no ID with an error", id)
			}
			if got := len(deploymentRows(t, database)); got != 0 {
				t.Errorf("deployments after the refusal = %d, want 0", got)
			}
		})
	}
}

func TestCreateDeploymentRefusesTooManyUnfinishedDeployments(t *testing.T) {
	database := ensureTestSchema(t)

	for number := 1; number <= 3; number++ {
		if _, err := database.CreateDeployment(t.Context(), "my-site", validManifestTest(), manifest.DefaultLimits(), 3); err != nil {
			t.Fatalf("CreateDeployment() number %d error = %v, want nil", number, err)
		}
	}
	id, err := database.CreateDeployment(t.Context(), "my-site", validManifestTest(), manifest.DefaultLimits(), 3)

	want := `project "my-site" has 3 unfinished deployments and the limit is 3, so finish or cancel one first`
	if err == nil || err.Error() != want {
		t.Errorf("CreateDeployment() error = %v, want %q", err, want)
	}
	if id != "" {
		t.Errorf("CreateDeployment() = %q, want no ID with an error", id)
	}
	if got := len(deploymentRows(t, database)); got != 3 {
		t.Errorf("deployments after the refusal = %d, want 3", got)
	}
}

func TestCreateDeploymentCountsOnlyUnfinishedStates(t *testing.T) {
	// Write the states out, so a change of the rule shows here. ADR 0002 calls
	// created and uploading the live states, and ADR 0004 limits them.
	tests := []struct {
		state       string
		wantRefused bool
	}{
		{state: "created", wantRefused: true},
		{state: "uploading", wantRefused: true},
		{state: "ready"},
		{state: "failed"},
		{state: "cancelled"},
		{state: "expired"},
		{state: "pruned"},
	}

	for _, test := range tests {
		t.Run(test.state, func(t *testing.T) {
			database := ensureTestSchema(t)
			insertUnfinishedTest(t, database, "my-site", 1, test.state)

			_, err := database.CreateDeployment(t.Context(), "my-site", validManifestTest(), manifest.DefaultLimits(), 1)

			if test.wantRefused && err == nil {
				t.Errorf("CreateDeployment() with one %s deployment and a limit of 1 error = nil, want a refusal", test.state)
			}
			if !test.wantRefused && err != nil {
				t.Errorf("CreateDeployment() with one %s deployment and a limit of 1 error = %v, want nil", test.state, err)
			}
		})
	}
}

func TestCreateDeploymentCountsOnlyTheProjectItself(t *testing.T) {
	database := ensureTestSchema(t)
	execTest(t, database, "INSERT INTO projects (id) VALUES ('other-site')")
	insertUnfinishedTest(t, database, "other-site", 1, "created")
	insertUnfinishedTest(t, database, "other-site", 2, "uploading")

	if _, err := database.CreateDeployment(t.Context(), "my-site", validManifestTest(), manifest.DefaultLimits(), 1); err != nil {
		t.Errorf("CreateDeployment() beside another project's unfinished deployments error = %v, want nil", err)
	}
}

func TestCreateDeploymentKeepsEachProjectApart(t *testing.T) {
	database := ensureTestSchema(t)
	execTest(t, database, "INSERT INTO projects (id) VALUES ('other-site')")

	// Expect each project to hold its own count, so a limit of 1 lets both create.
	first, err := database.CreateDeployment(t.Context(), "my-site", validManifestTest(), manifest.DefaultLimits(), 1)
	if err != nil {
		t.Fatalf("CreateDeployment() for my-site error = %v, want nil", err)
	}
	second, err := database.CreateDeployment(t.Context(), "other-site", validManifestTest(), manifest.DefaultLimits(), 1)
	if err != nil {
		t.Fatalf("CreateDeployment() for other-site error = %v, want nil", err)
	}

	projects := map[string]string{}
	for _, row := range deploymentRows(t, database) {
		projects[row.id] = row.projectID
	}
	if projects[first] != "my-site" || projects[second] != "other-site" {
		t.Errorf("projects of the deployments = %v, want %s in my-site and %s in other-site", projects, first, second)
	}

	_, err = database.CreateDeployment(t.Context(), "other-site", validManifestTest(), manifest.DefaultLimits(), 1)
	want := `project "other-site" has 1 unfinished deployments and the limit is 1, so finish or cancel one first`
	if err == nil || err.Error() != want {
		t.Errorf("CreateDeployment() for other-site at its limit error = %v, want %q", err, want)
	}
}

func TestCreateDeploymentRefusesEveryCreateWhenTheLimitIsNotPositive(t *testing.T) {
	// Expect a limit of zero or less to fail closed, so a wrong setting never
	// lets deployments through.
	tests := []struct {
		name          string
		maxUnfinished int
		want          string
	}{
		{name: "zero", maxUnfinished: 0, want: `project "my-site" has 0 unfinished deployments and the limit is 0, so finish or cancel one first`},
		{name: "negative", maxUnfinished: -1, want: `project "my-site" has 0 unfinished deployments and the limit is -1, so finish or cancel one first`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			database := ensureTestSchema(t)

			_, err := database.CreateDeployment(t.Context(), "my-site", validManifestTest(), manifest.DefaultLimits(), test.maxUnfinished)

			if err == nil || err.Error() != test.want {
				t.Errorf("CreateDeployment() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCreateDeploymentAcceptsAgainOnceOneIsFinished(t *testing.T) {
	database := ensureTestSchema(t)
	first, err := database.CreateDeployment(t.Context(), "my-site", validManifestTest(), manifest.DefaultLimits(), 1)
	if err != nil {
		t.Fatalf("CreateDeployment() error = %v, want nil", err)
	}
	if _, err := database.CreateDeployment(t.Context(), "my-site", validManifestTest(), manifest.DefaultLimits(), 1); err == nil {
		t.Fatalf("CreateDeployment() at the limit error = nil, want a refusal")
	}

	execTest(t, database, "UPDATE deployments SET state = 'cancelled' WHERE id = '"+first+"'")

	if _, err := database.CreateDeployment(t.Context(), "my-site", validManifestTest(), manifest.DefaultLimits(), 1); err != nil {
		t.Errorf("CreateDeployment() after a cancel error = %v, want nil", err)
	}
}

func TestCreateDeploymentReturnsContextError(t *testing.T) {
	database := ensureTestSchema(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	id, err := database.CreateDeployment(ctx, "my-site", validManifestTest(), manifest.DefaultLimits(), 10)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("CreateDeployment() error = %v, want it to wrap %v", err, context.Canceled)
	}
	if id != "" {
		t.Errorf("CreateDeployment() = %q, want no ID with an error", id)
	}
	if got := len(deploymentRows(t, database)); got != 0 {
		t.Errorf("deployments after the refusal = %d, want 0", got)
	}
}

func TestCreateDeploymentKeepsTheLimitUnderConcurrentCalls(t *testing.T) {
	const calls = 30
	const limit = 10
	database := ensureTestSchema(t)

	var group sync.WaitGroup
	results := make(chan error, calls)
	for range calls {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := database.CreateDeployment(t.Context(), "my-site", validManifestTest(), manifest.DefaultLimits(), limit)
			results <- err
		}()
	}
	group.Wait()
	close(results)

	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		}
	}
	if accepted != limit {
		t.Errorf("accepted calls = %d, want %d", accepted, limit)
	}
	if got := len(deploymentRows(t, database)); got != limit {
		t.Errorf("deployments = %d, want %d", got, limit)
	}
}

func TestCancelDeploymentCancelsAnUnfinishedDeployment(t *testing.T) {
	for _, state := range []deployment.State{deployment.Created, deployment.Uploading} {
		t.Run(string(state), func(t *testing.T) {
			database := ensureTestSchema(t)
			execTest(t, database, "INSERT INTO projects (id) VALUES ('other-site')")
			if err := insertDeploymentTest(t.Context(), database, deploymentIDTest, "my-site", string(state), `{"kept":true}`); err != nil {
				t.Fatalf("insert deployment: %v", err)
			}
			if err := insertDeploymentTest(t.Context(), database, otherDeploymentIDTest, "other-site", string(state), "{}"); err != nil {
				t.Fatalf("insert other deployment: %v", err)
			}

			before := time.Now().Unix()
			err := database.CancelDeployment(t.Context(), deploymentIDTest)
			after := time.Now().Unix()

			if err != nil {
				t.Fatalf("CancelDeployment() error = %v, want nil", err)
			}
			rows := deploymentRows(t, database)
			if len(rows) != 2 {
				t.Fatalf("deployments = %d, want 2", len(rows))
			}
			cancelled, other := rows[0], rows[1]
			if cancelled.id != deploymentIDTest {
				t.Fatalf("first row ID = %q, want %q", cancelled.id, deploymentIDTest)
			}
			if cancelled.state != "cancelled" {
				t.Errorf("state = %q, want %q", cancelled.state, "cancelled")
			}
			if cancelled.updatedAt < before || cancelled.updatedAt > after {
				t.Errorf("updated_at = %d, want between %d and %d", cancelled.updatedAt, before, after)
			}
			if cancelled.projectID != "my-site" || cancelled.manifest != `{"kept":true}` {
				t.Errorf("project and manifest = %q and %q, want them unchanged", cancelled.projectID, cancelled.manifest)
			}
			if other.state != string(state) || other.updatedAt != 1 {
				t.Errorf("deployment of another project = state %q, updated_at %d, want %q and 1", other.state, other.updatedAt, state)
			}
		})
	}
}

func TestCancelDeploymentRefusesAStateThatCannotMove(t *testing.T) {
	// The states are written out, so a new state needs a decision here.
	for _, state := range []string{"ready", "failed", "expired", "pruned"} {
		t.Run(state, func(t *testing.T) {
			database := ensureTestSchema(t)
			if err := insertDeploymentTest(t.Context(), database, deploymentIDTest, "my-site", state, "{}"); err != nil {
				t.Fatalf("insert deployment: %v", err)
			}

			err := database.CancelDeployment(t.Context(), deploymentIDTest)

			want := fmt.Sprintf(`cancel deployment: state conflict: a deployment cannot move from %q to "cancelled"`, state)
			if err == nil || err.Error() != want {
				t.Errorf("CancelDeployment() error = %v, want %q", err, want)
			}
			if !errors.Is(err, deployment.ErrStateConflict) {
				t.Errorf("CancelDeployment() error = %v, want it to wrap %v", err, deployment.ErrStateConflict)
			}
			rows := deploymentRows(t, database)
			if len(rows) != 1 || rows[0].state != state || rows[0].updatedAt != 1 {
				t.Errorf("rows after the refusal = %+v, want one %q row with updated_at 1", rows, state)
			}
		})
	}
}

func TestCancelDeploymentAcceptsARepeatAndChangesNothing(t *testing.T) {
	database := ensureTestSchema(t)
	if err := insertDeploymentTest(t.Context(), database, deploymentIDTest, "my-site", "cancelled", "{}"); err != nil {
		t.Fatalf("insert deployment: %v", err)
	}

	if err := database.CancelDeployment(t.Context(), deploymentIDTest); err != nil {
		t.Errorf("CancelDeployment() error = %v, want nil", err)
	}

	rows := deploymentRows(t, database)
	if len(rows) != 1 || rows[0].state != "cancelled" || rows[0].updatedAt != 1 {
		t.Errorf("rows after the repeat = %+v, want one cancelled row with updated_at 1", rows)
	}
}

func TestCancelDeploymentCancelsADeploymentThatCreateDeploymentMade(t *testing.T) {
	database := ensureTestSchema(t)
	id, err := database.CreateDeployment(t.Context(), "my-site", validManifestTest(), manifest.DefaultLimits(), 10)
	if err != nil {
		t.Fatalf("CreateDeployment() error = %v, want nil", err)
	}

	for attempt := 1; attempt <= 2; attempt++ {
		if err := database.CancelDeployment(t.Context(), id); err != nil {
			t.Errorf("CancelDeployment() attempt %d error = %v, want nil", attempt, err)
		}
		rows := deploymentRows(t, database)
		if len(rows) != 1 || rows[0].id != id || rows[0].state != "cancelled" {
			t.Errorf("rows after attempt %d = %+v, want one cancelled row with ID %q", attempt, rows, id)
		}
	}
}

func TestCancelDeploymentRejectsAMissingDeployment(t *testing.T) {
	database := ensureTestSchema(t)
	if err := insertDeploymentTest(t.Context(), database, otherDeploymentIDTest, "my-site", "created", "{}"); err != nil {
		t.Fatalf("insert deployment: %v", err)
	}

	err := database.CancelDeployment(t.Context(), deploymentIDTest)

	want := fmt.Sprintf("deployment %q does not exist", deploymentIDTest)
	if err == nil || err.Error() != want {
		t.Errorf("CancelDeployment() error = %v, want %q", err, want)
	}
	rows := deploymentRows(t, database)
	if len(rows) != 1 || rows[0].state != "created" {
		t.Errorf("rows after the refusal = %+v, want the other deployment unchanged", rows)
	}
}

func TestCancelDeploymentRejectsAnIDThatIsNotWellFormed(t *testing.T) {
	database := ensureTestSchema(t)
	if err := insertDeploymentTest(t.Context(), database, deploymentIDTest, "my-site", "created", "{}"); err != nil {
		t.Fatalf("insert deployment: %v", err)
	}
	if err := insertDeploymentTest(t.Context(), database, otherDeploymentIDTest, "my-site", "uploading", "{}"); err != nil {
		t.Fatalf("insert other deployment: %v", err)
	}

	for _, test := range []struct {
		name string
		id   string
	}{
		{name: "empty", id: ""},
		{name: "NUL byte", id: strings.Repeat("a", 15) + "\x00" + strings.Repeat("a", 16)},
		{name: "quote form", id: "x' OR '1'='1" + strings.Repeat("a", 20)},
		{name: "parent segment", id: "../" + strings.Repeat("a", 29)},
		{name: "uppercase", id: strings.ToUpper(deploymentIDTest)},
		{name: "oversized", id: strings.Repeat("a", 10_000)},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := database.CancelDeployment(t.Context(), test.id)

			want := "deployment ID must be 32 lowercase hexadecimal characters"
			if err == nil || err.Error() != want {
				t.Errorf("CancelDeployment(%.40q) error = %v, want %q", test.id, err, want)
			}
			rows := deploymentRows(t, database)
			if len(rows) != 2 || rows[0].state != "created" || rows[1].state != "uploading" {
				t.Errorf("rows after the refusal = %+v, want both unchanged", rows)
			}
		})
	}
}

func TestCancelDeploymentLeavesOtherDeploymentsAlone(t *testing.T) {
	database := ensureTestSchema(t)
	execTest(t, database, "INSERT INTO projects (id) VALUES ('other-site')")
	for number, project := range []string{"my-site", "my-site", "other-site"} {
		insertUnfinishedTest(t, database, project, number+1, "created")
	}

	if err := database.CancelDeployment(t.Context(), fmt.Sprintf("%032x", 2)); err != nil {
		t.Fatalf("CancelDeployment() error = %v, want nil", err)
	}

	var states []string
	for _, row := range deploymentRows(t, database) {
		states = append(states, row.state)
	}
	if got, want := strings.Join(states, ","), "created,cancelled,created"; got != want {
		t.Errorf("states = %q, want %q", got, want)
	}
}

func TestCancelDeploymentFreesRoomUnderTheUnfinishedLimit(t *testing.T) {
	database := ensureTestSchema(t)
	id, err := database.CreateDeployment(t.Context(), "my-site", validManifestTest(), manifest.DefaultLimits(), 1)
	if err != nil {
		t.Fatalf("CreateDeployment() error = %v, want nil", err)
	}
	if _, err := database.CreateDeployment(t.Context(), "my-site", validManifestTest(), manifest.DefaultLimits(), 1); err == nil {
		t.Fatal("CreateDeployment() at the limit error = nil, want an error")
	}

	if err := database.CancelDeployment(t.Context(), id); err != nil {
		t.Fatalf("CancelDeployment() error = %v, want nil", err)
	}

	if _, err := database.CreateDeployment(t.Context(), "my-site", validManifestTest(), manifest.DefaultLimits(), 1); err != nil {
		t.Errorf("CreateDeployment() after a cancel error = %v, want nil", err)
	}
}

func TestCancelDeploymentReturnsContextError(t *testing.T) {
	database := ensureTestSchema(t)
	if err := insertDeploymentTest(t.Context(), database, deploymentIDTest, "my-site", "created", "{}"); err != nil {
		t.Fatalf("insert deployment: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := database.CancelDeployment(ctx, deploymentIDTest)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("CancelDeployment() error = %v, want it to wrap %v", err, context.Canceled)
	}
	rows := deploymentRows(t, database)
	if len(rows) != 1 || rows[0].state != "created" {
		t.Errorf("rows after the refusal = %+v, want the deployment unchanged", rows)
	}
}
