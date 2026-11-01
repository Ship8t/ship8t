package storage

import (
	"os/exec"
	"testing"
)

func TestShellQuoteRoundTripsThroughShell(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "plain", text: "data"},
		{name: "space", text: "my data"},
		{name: "single quote", text: "it's"},
		{name: "only single quotes", text: "'''"},
		{name: "shell metacharacters", text: "a;b&c|d$(x)`y`*?"},
		{name: "leading dash", text: "-n"},
		{name: "empty", text: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Build the shell string from the quoted text on purpose, because the
			// round trip is what proves the quoting.
			output, err := exec.Command("sh", "-c", "printf %s "+shellQuote(test.text)).Output()
			if err != nil {
				t.Fatalf("sh failed for %q: %v", shellQuote(test.text), err)
			}
			if string(output) != test.text {
				t.Errorf("shell read %q, want %q", output, test.text)
			}
		})
	}
}

func TestChmodCommand(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		path        string
		wantCommand string
		wantOK      bool
	}{
		{name: "plain path", mode: "700", path: "/srv/ship8t", wantCommand: "chmod -- 700 '/srv/ship8t'", wantOK: true},
		{name: "relative path with a dash", mode: "700", path: "-data", wantCommand: "chmod -- 700 '-data'", wantOK: true},
		{name: "single quote", mode: "700", path: "/srv/it's", wantCommand: `chmod -- 700 '/srv/it'\''s'`, wantOK: true},
		{name: "non ASCII printable", mode: "700", path: "/srv/données", wantCommand: "chmod -- 700 '/srv/données'", wantOK: true},
		{name: "file mode", mode: "600", path: "/srv/ship8t/ship8t.db", wantCommand: "chmod -- 600 '/srv/ship8t/ship8t.db'", wantOK: true},
		{name: "file mode with a control character", mode: "600", path: "/srv/a\nb"},
		{name: "newline", mode: "700", path: "/srv/a\nb"},
		{name: "escape", mode: "700", path: "/srv/a\x1bb"},
		{name: "right to left override", mode: "700", path: "/srv/a\u202eb"},
		{name: "invalid UTF-8", mode: "700", path: "/srv/a\xffb"},
		{name: "no break space", mode: "700", path: "/srv/a\u00a0b"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command, ok := chmodCommand(test.mode, test.path)
			if ok != test.wantOK || command != test.wantCommand {
				t.Errorf("chmodCommand(%q, %q) = %q, %v, want %q, %v", test.mode, test.path, command, ok, test.wantCommand, test.wantOK)
			}
		})
	}
}
