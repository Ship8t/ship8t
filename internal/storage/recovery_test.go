package storage

import "testing"

// TestRecoveryAfterInterruption lists the interruption points of ADR 0003 that
// the storage code must survive. Turn on a case by replacing its skip with an
// injected failure at that point, a restart, and the check that the case names.
// Every case must also leave the active release unchanged, except activation,
// which leaves the old release or the new one.
func TestRecoveryAfterInterruption(t *testing.T) {
	tests := []struct {
		name  string
		needs string
		check string
	}{
		{
			name:  "crash between the rename into releases and the ready commit",
			needs: "finalize with a failure injected after the rename, and recovery at startup",
			check: "recovery re-verifies the files against the manifest, completes the move to ready, and leaves the active release unchanged",
		},
		{
			name:  "staging directory of a terminal or unknown deployment",
			needs: "recovery at startup",
			check: "recovery deletes the directory and leaves the active release unchanged",
		},
		{
			name:  "releases directory with no database row",
			needs: "recovery at startup",
			check: "recovery moves the directory to quarantine, never serves or deletes it, and leaves the active release unchanged",
		},
		{
			name:  "ready deployment whose directory is missing",
			needs: "recovery at startup and the degraded mark of a project",
			check: "recovery logs an error, marks the project degraded, repairs nothing, and leaves the active release unchanged",
		},
		{
			name:  "leftover entries in trash",
			needs: "recovery at startup",
			check: "recovery finishes the removal and leaves the active release unchanged",
		},
		{
			name:  "crash during activation",
			needs: "activation with a failure injected inside the transaction",
			check: "the pointer names the old release or the new one, agrees with the activation log, and no request sees a mix",
		},
		{
			name:  "prune interrupted after the move to trash and before the pruned mark",
			needs: "pruning with a failure injected after the move, and recovery at startup",
			check: "recovery follows the rule that the maintainer sets, because ADR 0003 gives none for a ready deployment whose directory sits in trash, and the active release is unchanged",
		},
		{
			name:  "prune interrupted after the pruned mark and before the delete",
			needs: "pruning with a failure injected after the mark, and recovery at startup",
			check: "recovery finishes the removal from trash, the deployment stays pruned, and the active release is unchanged",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Skipf("pending: needs %s, then check that %s", test.needs, test.check)
		})
	}
}
