package storage

import "testing"

// TestConcurrentActivation lists the races between activation operations of one
// project that ADR 0002 and ADR 0003 settle. Turn on a case by replacing its
// skip with goroutines that start together, and run it with the race detector,
// as make test does. Activation runs in one transaction per project, so the
// last committed one wins and no request sees a mix of two releases.
func TestConcurrentActivation(t *testing.T) {
	tests := []struct {
		name  string
		needs string
		check string
	}{
		{
			name:  "two publishes of different ready releases",
			needs: "activation by publish with a database transaction",
			check: "both succeed, the pointer names the release of the last log row, and each log row names the previous release of the one before it",
		},
		{
			name:  "two publishes of the same ready release that is not active",
			needs: "activation by publish with a database transaction",
			check: "both succeed and exactly one log row is written, because publishing the active release is a no-op",
		},
		{
			name:  "rollback that races a newer publish with expected_active",
			needs: "activation by publish and rollback with the expected_active guard",
			check: "the rollback either lands before the publish or fails with active_mismatch and changes nothing, and the pointer and the log agree",
		},
		{
			name:  "two rollbacks with the same expected_active",
			needs: "activation by rollback with the expected_active guard",
			check: "exactly one succeeds, the other fails with active_mismatch and changes nothing, and the log holds one new row",
		},
		{
			name:  "prune that races the activation of the same release",
			needs: "activation by publish and rollback, and pruning",
			check: "either the activation lands first and the prune is refused, or the prune lands first and the activation fails with a conflict, and the pointer never names a pruned release",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Skipf("pending: needs %s, then check that %s", test.needs, test.check)
		})
	}
}

// TestPruneRefusesActiveRelease lists the attempts to prune the active release,
// which ADR 0003 asks to test as a security case. Turn on a case by replacing
// its skip with the real attempt.
func TestPruneRefusesActiveRelease(t *testing.T) {
	tests := []struct {
		name  string
		needs string
		check string
	}{
		{
			name:  "retention lists the active release among its candidates",
			needs: "retention and pruning",
			check: "the prune is refused, the release stays ready with its content, and the pointer is unchanged",
		},
		{
			name:  "active release is older than the 10 most recent ready releases",
			needs: "retention and pruning",
			check: "retention keeps the active release and the 10 most recent ready releases, so 11 releases remain and the active one is not pruned",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Skipf("pending: needs %s, then check that %s", test.needs, test.check)
		})
	}
}
