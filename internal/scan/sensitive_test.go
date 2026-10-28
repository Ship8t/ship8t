package scan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSensitiveByName(t *testing.T) {
	tests := []struct {
		name string
		file string
		want bool
	}{
		{"env file", ".env", true},
		{"env file with a suffix", ".env.local", true},
		{"env file in a directory", "config/.env.production", true},
		{"rsa private key", "id_rsa", true},
		{"ed25519 private key", "id_ed25519", true},
		{"ecdsa private key", "id_ecdsa", true},
		{"private key in a directory", "keys/id_rsa", true},
		{"pkcs12 bundle", "client.p12", true},
		{"pfx bundle", "client.pfx", true},
		{"bundle in a directory", "certs/site.pfx", true},
		{"private key in capitals", "ID_RSA", true},
		{"bundle in capitals", "CLIENT.PFX", true},
		{"env file in capitals", ".ENV", true},

		{"rsa public key", "id_rsa.pub", false},
		{"ed25519 public key", "id_ed25519.pub", false},
		{"ecdsa public key", "id_ecdsa.pub", false},
		{"name that starts like a private key", "id_rsa_notes.html", false},
		{"page about the environment", "environment.html", false},
		{"env without a dot", "env", false},
		{"env in the middle of a name", "my.env", false},
		{"bundle extension in the middle", "notes.pfx.txt", false},
		{"directory named like a bundle", "site.p12/index.html", false},
		{"directory named like a key", "id_rsa/index.html", false},
		{"regular file", "index.html", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := sensitiveByName(test.file); got != test.want {
				t.Errorf("sensitiveByName(%q) = %v, want %v", test.file, got, test.want)
			}
		})
	}
}

func TestScanRejectsSensitiveFiles(t *testing.T) {
	tests := []struct {
		name    string
		files   []string
		options Options
		want    []string
	}{
		{
			name:  "private key",
			files: []string{"index.html", "id_rsa"},
			want:  []string{"id_rsa"},
		},
		{
			name:  "bundle in a directory",
			files: []string{"index.html", "certs/site.pfx"},
			want:  []string{"certs/site.pfx"},
		},
		{
			name:    "env file that an include pattern added back",
			files:   []string{"index.html", ".env"},
			options: Options{Include: []string{".env"}},
			want:    []string{".env"},
		},
		{
			name:    "env file in a directory that include opened",
			files:   []string{"index.html", ".cache/.env.local"},
			options: Options{Include: []string{".cache"}},
			want:    []string{".cache/.env.local"},
		},
		{
			name:  "every sensitive file",
			files: []string{"index.html", "id_rsa", "keys/client.p12", "keys/id_ed25519"},
			want:  []string{"id_rsa", "keys/client.p12", "keys/id_ed25519"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			for _, path := range test.files {
				writeFile(t, root, path)
			}

			files, err := Scan(root, test.options)
			if err == nil {
				t.Fatalf("Scan() = %v, nil error, want an error", files)
			}
			message := err.Error()
			for _, path := range test.want {
				if !strings.Contains(message, `scan "`+path+`":`) {
					t.Errorf("Scan() error = %q, want it to name %q", message, path)
				}
				if !strings.Contains(message, `ignore rule "`+path+`"`) {
					t.Errorf("Scan() error = %q, want it to show the ignore rule %q", message, path)
				}
			}
			if strings.Contains(message, "index.html") {
				t.Errorf("Scan() error = %q, want it not to name index.html", message)
			}
		})
	}
}

func TestScanAcceptsSensitiveFilesThatTheUserAllowsOrIgnores(t *testing.T) {
	tests := []struct {
		name    string
		files   []string
		options Options
		want    []string
	}{
		{
			name:    "allow sensitive files",
			files:   []string{"index.html", "id_rsa", "certs/site.pfx"},
			options: Options{AllowSensitiveFiles: true},
			want:    []string{"certs/site.pfx", "id_rsa", "index.html"},
		},
		{
			name:    "ignore rule that names the file",
			files:   []string{"index.html", "id_rsa"},
			options: Options{Ignore: []string{"id_rsa"}},
			want:    []string{"index.html"},
		},
		{
			name:    "ignore rule that names the directory",
			files:   []string{"index.html", "certs/site.pfx", "certs/id_rsa"},
			options: Options{Ignore: []string{"certs/"}},
			want:    []string{"index.html"},
		},
		{
			name:  "env file that a default rule removes",
			files: []string{"index.html", ".env", ".env.local"},
			want:  []string{"index.html"},
		},
		{
			name:  "public key",
			files: []string{"index.html", "id_rsa.pub"},
			want:  []string{"id_rsa.pub", "index.html"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertScan(t, test.files, test.options, test.want)
		})
	}
}

func TestScanSensitiveErrorListsFilesInOrder(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"z/id_rsa", "a/id_rsa", "index.html"} {
		writeFile(t, root, path)
	}

	_, err := Scan(root, Options{})
	if err == nil {
		t.Fatal("Scan() = nil error, want an error")
	}
	lines := strings.Split(err.Error(), "\n")
	if len(lines) != 2 {
		t.Fatalf("Scan() error = %q, want one line for each sensitive file", err)
	}
	if !strings.HasPrefix(lines[0], `scan "a/id_rsa":`) || !strings.HasPrefix(lines[1], `scan "z/id_rsa":`) {
		t.Errorf("Scan() error lines = %q, want them sorted by path", lines)
	}
}

func TestScanSensitiveErrorEscapesNewlinesInNames(t *testing.T) {
	root := t.TempDir()
	// A name with a newline is not valid on every platform, so skip where the
	// filesystem refuses it.
	directory := "fake\nscan \"index.html\": ok"
	if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
		t.Skipf("file names with a newline are not available: %v", err)
	}
	writeFile(t, root, directory+"/id_rsa")

	_, err := Scan(root, Options{})
	if err == nil {
		t.Fatal("Scan() = nil error, want an error")
	}
	if lines := strings.Split(err.Error(), "\n"); len(lines) != 1 {
		t.Errorf("Scan() error = %q, want a single line, so a file name cannot forge extra lines", err)
	}
}
