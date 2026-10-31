package deployment

import (
	"errors"
	"strings"
	"testing"
)

// TestTransition covers every ordered pair of the seven states against the
// table of ADR 0002, which the test spells out apart from the code under test.
func TestTransition(t *testing.T) {
	states := []State{"created", "uploading", "ready", "failed", "cancelled", "expired", "pruned"}

	allowed := map[[2]State]bool{
		{"created", "uploading"}:   true,
		{"created", "ready"}:       true,
		{"created", "failed"}:      true,
		{"created", "cancelled"}:   true,
		{"created", "expired"}:     true,
		{"uploading", "ready"}:     true,
		{"uploading", "failed"}:    true,
		{"uploading", "cancelled"}: true,
		{"uploading", "expired"}:   true,
		{"ready", "pruned"}:        true,
	}

	for _, from := range states {
		for _, to := range states {
			name := string(from) + " to " + string(to)
			t.Run(name, func(t *testing.T) {
				changed, err := Transition(from, to)

				switch {
				case from == to:
					if err != nil || changed {
						t.Fatalf("Transition(%q, %q) = %v, %v, want a no-op", from, to, changed, err)
					}
				case allowed[[2]State{from, to}]:
					if err != nil || !changed {
						t.Fatalf("Transition(%q, %q) = %v, %v, want an allowed change", from, to, changed, err)
					}
				default:
					if !errors.Is(err, ErrStateConflict) || changed {
						t.Fatalf("Transition(%q, %q) = %v, %v, want ErrStateConflict", from, to, changed, err)
					}
				}
			})
		}
	}
}

func TestTransitionRejectsUnknownStates(t *testing.T) {
	tests := []struct {
		name string
		from State
		to   State
	}{
		{name: "repeat of an unknown state", from: "archived", to: "archived"},
		{name: "repeat of the empty state", from: "", to: ""},
		{name: "known state to an unknown state", from: "created", to: "archived"},
		{name: "unknown state to a known state", from: "archived", to: "ready"},
		{name: "wrong letter case", from: "Created", to: "Ready"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed, err := Transition(test.from, test.to)
			if !errors.Is(err, ErrStateConflict) || changed {
				t.Fatalf("Transition(%q, %q) = %v, %v, want ErrStateConflict", test.from, test.to, changed, err)
			}
		})
	}
}

func TestTransitionErrorNamesBothStates(t *testing.T) {
	_, err := Transition(Ready, Uploading)
	if err == nil {
		t.Fatal("Transition(ready, uploading) succeeded, want an error")
	}

	for _, want := range []string{`"ready"`, `"uploading"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %s", err, want)
		}
	}
}

func TestTransitionErrorCapsAnUnknownState(t *testing.T) {
	long := State(strings.Repeat("x", 1000))

	tests := []struct {
		name string
		from State
		to   State
	}{
		{name: "long source state", from: long, to: Ready},
		{name: "long target state", from: Ready, to: long},
		{name: "long source and target state", from: long, to: long},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Transition(test.from, test.to)
			if err == nil {
				t.Fatalf("Transition(%.8q, %.8q) succeeded, want an error", test.from, test.to)
			}

			if !strings.Contains(err.Error(), strings.Repeat("x", 64)) {
				t.Errorf("error %q does not keep the first 64 characters", err)
			}
			if strings.Contains(err.Error(), strings.Repeat("x", 65)) {
				t.Errorf("error is %d bytes long, want the unknown state capped at 64 characters", len(err.Error()))
			}
		})
	}
}
