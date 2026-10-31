// Package deployment defines the states of a deployment and the moves between
// them, as ADR 0002 sets out.
package deployment

import (
	"errors"
	"fmt"
	"slices"
)

// State is the state of a deployment. It says how far the deployment got and
// never whether the deployment is active.
type State string

// These are the states of ADR 0002. Ready is the only state that has content a
// release can serve, and Failed, Cancelled, Expired, and Pruned are terminal.
const (
	Created   State = "created"
	Uploading State = "uploading"
	Ready     State = "ready"
	Failed    State = "failed"
	Cancelled State = "cancelled"
	Expired   State = "expired"
	Pruned    State = "pruned"
)

// ErrStateConflict reports a move between states that ADR 0002 does not allow.
// The server answers it with 409 and the code state_conflict.
var ErrStateConflict = errors.New("state conflict")

// states lists every state, so a repeat of an unknown state is not a no-op.
var states = []State{Created, Uploading, Ready, Failed, Cancelled, Expired, Pruned}

// allowedMoves lists the states that a deployment may move to from each state.
// A state without an entry is terminal.
var allowedMoves = map[State][]State{
	Created:   {Uploading, Ready, Failed, Cancelled, Expired},
	Uploading: {Ready, Failed, Cancelled, Expired},
	Ready:     {Pruned},
}

// Transition checks a move of a deployment from one state to another and
// reports whether the state changes. A move to the current state succeeds and
// changes nothing, so a repeated request is safe. Any other move that the table
// does not list fails with ErrStateConflict. It does not check whether the
// deployment is active, because activation belongs to the code that owns the
// active pointer.
func Transition(from, to State) (changed bool, err error) {
	if from == to && slices.Contains(states, from) {
		return false, nil
	}
	if slices.Contains(allowedMoves[from], to) {
		return true, nil
	}

	// Cap the quoted names, because a state read from storage is untrusted.
	return false, fmt.Errorf("%w: a deployment cannot move from %.64q to %.64q", ErrStateConflict, from, to)
}
