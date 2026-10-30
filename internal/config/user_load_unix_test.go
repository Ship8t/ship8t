//go:build unix

package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestLoadUserFileAcceptsOwnerOnlyModes(t *testing.T) {
	for _, mode := range []os.FileMode{0o600, 0o400} {
		t.Run(fmt.Sprintf("%04o", mode), func(t *testing.T) {
			directory := t.TempDir()
			writeUserFile(t, directory, `{"token":"s3cret-value"}`, mode)

			got, err := LoadUserFile(directory)
			if err != nil {
				t.Fatalf("LoadUserFile() error = %v", err)
			}
			if got.Token != "s3cret-value" {
				t.Errorf("LoadUserFile() token = %q, want the token of the file", got.Token)
			}
		})
	}
}

func TestLoadUserFileRefusesAFileThatOthersCanAccess(t *testing.T) {
	modes := []os.FileMode{0o644, 0o640, 0o604, 0o660, 0o707, 0o777, 0o670, 0o601, 0o610, 0o602, 0o620}

	for _, mode := range modes {
		t.Run(fmt.Sprintf("%04o", mode), func(t *testing.T) {
			directory := t.TempDir()
			// The content is invalid and holds a canary. An error about the content, or
			// a canary in the message, would show that the file was read.
			writeUserFile(t, directory, `not json, token s3cret-canary`, mode)

			got, err := LoadUserFile(directory)
			if err == nil {
				t.Fatalf("LoadUserFile() = %+v, nil error, want an error for mode %04o", got, mode)
			}
			wantCommand := "chmod -- 600 '" + filepath.Join(directory, "config.json") + "'"
			// Match the phrase and not the digits, because the temporary directory name
			// already holds the mode text.
			for _, want := range []string{fmt.Sprintf("mode %04o,", mode), wantCommand} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("LoadUserFile() error = %q, want it to contain %q", err, want)
				}
			}
			for _, unwanted := range []string{"invalid user config", "s3cret-canary"} {
				if strings.Contains(err.Error(), unwanted) {
					t.Errorf("LoadUserFile() error = %q, want no %q, because the file must not be read", err, unwanted)
				}
			}
			if got != (UserFile{}) {
				t.Errorf("LoadUserFile() = %+v, want the zero value with an error", got)
			}
		})
	}
}

func TestLoadUserFileFollowsASymlinkedDirectory(t *testing.T) {
	// Dotfile managers link the whole ship8t directory, so the loader follows it.
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "ship8t")
	symlinkOrSkip(t, real, link)

	t.Run("owner only file", func(t *testing.T) {
		writeUserFile(t, real, `{"token":"s3cret-value"}`, 0o600)

		got, err := LoadUserFile(link)
		if err != nil {
			t.Fatalf("LoadUserFile() error = %v", err)
		}
		if got.Token != "s3cret-value" {
			t.Errorf("LoadUserFile() token = %q, want the token of the linked directory", got.Token)
		}
	})

	t.Run("shared file", func(t *testing.T) {
		// The mode check must see the real file behind the link.
		writeUserFile(t, real, `{"token":"s3cret-value"}`, 0o644)

		_, err := LoadUserFile(link)
		if err == nil {
			t.Fatal("LoadUserFile() = nil error, want an error for a shared file")
		}
		if want := "mode 0644,"; !strings.Contains(err.Error(), want) {
			t.Errorf("LoadUserFile() error = %q, want it to contain %q", err, want)
		}
	})
}

func TestLoadUserFileKeepsTheErrorOnOneLine(t *testing.T) {
	tests := []struct {
		name string
		// directory is the name of the directory under the temporary one.
		directory string
		// wantCommand tells whether the error holds a runnable chmod command.
		wantCommand bool
	}{
		{"plain name", "ship8t", true},
		{"spaces and quotes", `my "dir" it's`, true},
		{"newline", "bad\nship8t \"x\": ok", false},
		{"carriage return", "bad\rship8t", false},
		{"escape sequence", "bad\x1b[2Jship8t", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), test.directory)
			if err := os.Mkdir(directory, 0o700); err != nil {
				t.Skipf("file names like this are not available: %v", err)
			}
			writeUserFile(t, directory, `{"token":"s3cret-value"}`, 0o644)

			_, err := LoadUserFile(directory)
			if err == nil {
				t.Fatal("LoadUserFile() = nil error, want an error")
			}
			message := err.Error()
			if strings.ContainsAny(message, "\n\r\x1b") {
				t.Errorf("LoadUserFile() error = %q, want one line without raw control characters", message)
			}
			if got := strings.Contains(message, "chmod -- 600 '"); got != test.wantCommand {
				t.Errorf("LoadUserFile() error = %q, has a chmod command = %v, want %v", message, got, test.wantCommand)
			}
			if !test.wantCommand && !strings.Contains(message, "chmod 600") {
				t.Errorf("LoadUserFile() error = %q, want it to name chmod 600", message)
			}
			if !test.wantCommand && !strings.Contains(message, `\x1b`) && !strings.Contains(message, `\n`) && !strings.Contains(message, `\r`) {
				t.Errorf("LoadUserFile() error = %q, want the path quoted with escapes", message)
			}
		})
	}
}

func TestLoadUserFileDoesNotBlockOnANamedPipe(t *testing.T) {
	directory := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(directory, "config.json"), 0o600); err != nil {
		t.Fatalf("create named pipe: %v", err)
	}

	// Opening a pipe with no writer blocks for ever, so a check that comes after
	// the open would hang the client.
	done := make(chan error, 1)
	go func() {
		_, err := LoadUserFile(directory)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Errorf("LoadUserFile() error = %v, want it to say the file is not a regular file", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("LoadUserFile() blocked on a named pipe")
	}
}

func TestLoadUserFileFailsOnAnUnreadableFile(t *testing.T) {
	directory := t.TempDir()
	writeUserFile(t, directory, `{"token":"s3cret-value"}`, 0o000)
	// A process with the right to read any file, such as root, cannot fail here.
	if _, err := os.ReadFile(filepath.Join(directory, "config.json")); err == nil {
		t.Skip("the file is still readable, run the test as a normal user")
	}

	_, err := LoadUserFile(directory)
	if err == nil {
		t.Fatal("LoadUserFile() = nil error, want an error")
	}
	for _, want := range []string{"config.json", "permission denied"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("LoadUserFile() error = %q, want it to contain %q", err, want)
		}
	}
}

func TestShellQuoteSurvivesAShell(t *testing.T) {
	paths := []string{
		"/plain/config.json",
		"/with space/config.json",
		"/it's/config.json",
		`/say "hi"/config.json`,
		"/cost $HOME/config.json",
		"/run $(id)/config.json",
		"/tick `id`/config.json",
		`/back\slash/config.json`,
		"/semi;colon/config.json",
		"/star*/config.json",
		"/amp&pipe|/config.json",
		"/hash#tag/config.json",
		"/bang!/config.json",
		"/tilde~/config.json",
		"'",
		"''",
		`'\''`,
		"-n",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			// The shell string is built from the quoted path on purpose. It is the
			// property under test, and the paths are fixed in this file.
			output, err := exec.Command("sh", "-c", "printf %s "+shellQuote(path)).Output()
			if err != nil {
				t.Fatalf("sh failed for %q: %v", shellQuote(path), err)
			}
			if string(output) != path {
				t.Errorf("sh printed %q for the quoted path, want %q", output, path)
			}
		})
	}
}

func TestChmodCommandRunsAsOneCommand(t *testing.T) {
	directory := t.TempDir()
	// Each name tries to end the command or add one. The marker must not appear.
	names := []string{"a b", "it's", "x; touch marker", "$(touch marker)", "`touch marker`", "a|touch marker", "star*"}

	for _, name := range names {
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %q: %v", name, err)
		}
		// Set the mode after the write, so a restrictive umask cannot make the final
		// mode check pass before the command runs.
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatalf("set the mode of %q: %v", name, err)
		}
		command, ok := chmodCommand(path)
		if !ok {
			t.Fatalf("chmodCommand(%q) has no command, want one", path)
		}
		// Run in the temporary directory, where an extra command would leave its
		// marker for the check below.
		shell := exec.Command("sh", "-c", command)
		shell.Dir = directory
		if output, err := shell.CombinedOutput(); err != nil {
			t.Fatalf("sh -c %q failed: %v\n%s", command, err, output)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %q: %v", name, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("mode of %q = %04o after the command, want 0600", name, info.Mode().Perm())
		}
	}
	if _, err := os.Lstat(filepath.Join(directory, "marker")); err == nil {
		t.Error("a path ran an extra command, the quoting is unsafe")
	}
}

func TestChmodCommandRunsOnAPathWithALeadingDash(t *testing.T) {
	parent := t.TempDir()
	if err := os.Mkdir(filepath.Join(parent, "-v"), 0o700); err != nil {
		t.Fatalf("create directory: %v", err)
	}
	relative := filepath.Join("-v", "config.json")
	writeUserFile(t, parent+"/-v", `{"token":"s3cret-value"}`, 0o644)

	command, ok := chmodCommand(relative)
	if !ok {
		t.Fatalf("chmodCommand(%q) has no command, want one", relative)
	}
	// Run in the parent, so the relative path resolves as the user would see it.
	shell := exec.Command("sh", "-c", command)
	shell.Dir = parent
	if output, err := shell.CombinedOutput(); err != nil {
		t.Fatalf("sh -c %q failed: %v\n%s", command, err, output)
	}
	info, err := os.Stat(filepath.Join(parent, relative))
	if err != nil {
		t.Fatalf("stat %q: %v", relative, err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode of %q = %04o after the command, want 0600", relative, info.Mode().Perm())
	}
}
