package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name               string
		args               []string
		wantExitCode       int
		wantStdoutContains string
		wantStderrContains string
	}{
		{
			name:               "no arguments prints usage and fails",
			args:               nil,
			wantExitCode:       exitUsageError,
			wantStderrContains: "Usage: ship8td",
		},
		{
			name:               "help prints usage",
			args:               []string{"help"},
			wantExitCode:       exitSuccess,
			wantStdoutContains: "Usage: ship8td",
		},
		{
			name:               "long help flag prints usage",
			args:               []string{"--help"},
			wantExitCode:       exitSuccess,
			wantStdoutContains: "Usage: ship8td",
		},
		{
			name:               "short help flag prints usage",
			args:               []string{"-h"},
			wantExitCode:       exitSuccess,
			wantStdoutContains: "Usage: ship8td",
		},
		{
			name:               "version prints the server version",
			args:               []string{"version"},
			wantExitCode:       exitSuccess,
			wantStdoutContains: "ship8td ",
		},
		{
			name:               "unknown command fails and prints usage",
			args:               []string{"bogus"},
			wantExitCode:       exitUsageError,
			wantStderrContains: `unknown command "bogus"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			exitCode := run(test.args, &stdout, &stderr)

			if exitCode != test.wantExitCode {
				t.Errorf("exit code = %d, want %d", exitCode, test.wantExitCode)
			}
			if !strings.Contains(stdout.String(), test.wantStdoutContains) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), test.wantStdoutContains)
			}
			if !strings.Contains(stderr.String(), test.wantStderrContains) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), test.wantStderrContains)
			}
		})
	}
}
