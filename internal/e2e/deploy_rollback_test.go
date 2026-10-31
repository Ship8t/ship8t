package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ship8t/ship8t/internal/config"
	"github.com/ship8t/ship8t/internal/manifest"
	"github.com/ship8t/ship8t/internal/scan"
)

// firstVersion holds the files of the first version of the test site by
// slash-separated path. The default ignore rules keep the finder file and the
// config file out of the manifest. The ignore rule of the config file keeps the
// source map out, and its include rule adds the dot file back.
var firstVersion = map[string]string{
	"index.html":        "<h1>version 1</h1>\n",
	"assets/app.js":     "console.log('version 1');\n",
	"assets/app.js.map": "source map",
	".nojekyll":         "\n",
	".DS_Store":         "finder noise",
	"ship8t.json":       `{"project":"demo","ignore":["**/*.map"],"include":[".nojekyll"]}`,
}

// writeSite creates the files under a new directory and returns the directory.
func writeSite(t *testing.T, files map[string]string) string {
	t.Helper()
	directory := t.TempDir()
	for path, content := range files {
		full := filepath.Join(directory, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("create directory for %s: %v", path, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return directory
}

// checksum returns the SHA-256 checksum of content in lowercase hexadecimal.
// The test computes it here, apart from the code under test.
func checksum(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// pending skips a stage whose code does not exist yet and says what it needs.
func pending(t *testing.T, needs string) {
	t.Helper()
	t.Skipf("pending: needs %s", needs)
}

// TestDeployAndRollback walks through the stages of ADR 0001, section 11. Turn
// on a stage by replacing its call to pending with the real steps.
func TestDeployAndRollback(t *testing.T) {
	site := writeSite(t, firstVersion)

	t.Run("scan the first version into a manifest", func(t *testing.T) {
		// Read the rules from ship8t.json as the deploy command will.
		project, err := config.LoadProjectFile(site)
		if err != nil {
			t.Fatalf("LoadProjectFile() error = %v", err)
		}
		files, err := scan.Scan(site, scan.Options{Ignore: project.Ignore, Include: project.Include})
		if err != nil {
			t.Fatalf("Scan() error = %v", err)
		}
		described, err := scan.Describe(site, files, manifest.DefaultLimits())
		if err != nil {
			t.Fatalf("Describe() error = %v", err)
		}

		// Decode the manifest as the server does, so the client and the server
		// agree on the wire form.
		body, err := json.Marshal(manifest.Manifest{ManifestVersion: manifest.Version, Files: described})
		if err != nil {
			t.Fatalf("encode manifest: %v", err)
		}
		decoded, err := manifest.Decode(bytes.NewReader(body), manifest.DefaultLimits())
		if err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		want := []manifest.File{
			{Path: ".nojekyll", Size: int64(len(firstVersion[".nojekyll"])), SHA256: checksum(firstVersion[".nojekyll"])},
			{Path: "assets/app.js", Size: int64(len(firstVersion["assets/app.js"])), SHA256: checksum(firstVersion["assets/app.js"])},
			{Path: "index.html", Size: int64(len(firstVersion["index.html"])), SHA256: checksum(firstVersion["index.html"])},
		}
		if !reflect.DeepEqual(decoded.Files, want) {
			t.Errorf("manifest files = %v, want %v", decoded.Files, want)
		}
	})

	t.Run("start the server on random loopback ports", func(t *testing.T) {
		pending(t, "the server API, storage, and the ship8td serve command")
	})

	t.Run("deploy the first version through the client", func(t *testing.T) {
		pending(t, "the upload protocol and the ship8t deploy command")
	})

	t.Run("fetch the first version over HTTP", func(t *testing.T) {
		pending(t, "local site serving with project hostname routing")
	})

	t.Run("deploy a newer version and confirm it is active", func(t *testing.T) {
		pending(t, "atomic activation and the deployment history")
	})

	t.Run("roll back and confirm the earlier content is active again", func(t *testing.T) {
		pending(t, "the ship8t rollback command")
	})

	t.Run("fail a deployment and confirm the active release is unchanged", func(t *testing.T) {
		pending(t, "the deployment state machine and crash recovery")
	})
}
