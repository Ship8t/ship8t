package storage

import "testing"

// TestRecoveryAfterInterruption lists the rows of the recovery table of ADR
// 0003, the interruptions of its activation and prune sequences, and the rule
// of its history for an active release in the pruning directory. Turn on a case
// by replacing its skip with an injected failure at that point, a restart, and
// the check that the case names. Every case must also leave the active release
// unchanged, except activation, which leaves the old release or the new one.
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
			name:  "staging directory of a live deployment",
			needs: "recovery at startup and the idle expiry of a deployment",
			check: "recovery keeps the directory so the client can resume, idle expiry still applies, and the active release is unchanged",
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
			name:  "active release whose directory sits in pruning",
			needs: "recovery at startup, the degraded mark of a project, and the active pointer",
			check: "recovery finishes no prune, logs an error, marks the project degraded, repairs nothing, and leaves the active release unchanged",
		},
		{
			name:  "leftover entries in pruning",
			needs: "recovery at startup",
			check: "recovery finishes the removal of each entry that is not the active release, and leaves the active release unchanged",
		},
		{
			name:  "crash during activation",
			needs: "activation with a failure injected inside the transaction",
			check: "the pointer names the old release or the new one, agrees with the activation log, and no request sees a mix",
		},
		{
			name:  "prune interrupted after the move to pruning and before the pruned mark",
			needs: "pruning with a failure injected after the move, and recovery at startup",
			check: "recovery marks the deployment pruned in one transaction and deletes its entry from pruning, and the active release is unchanged",
		},
		{
			name:  "prune interrupted after the pruned mark and before the delete",
			needs: "pruning with a failure injected after the mark, and recovery at startup",
			check: "recovery finishes the removal from pruning, the deployment stays pruned, and the active release is unchanged",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Skipf("pending: needs %s, then check that %s", test.needs, test.check)
		})
	}
}
