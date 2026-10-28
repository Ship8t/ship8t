package scan

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

// sensitiveContentWindow is how many bytes from the start of a file the content
// check reads, from ADR 0001 section 7.
const sensitiveContentWindow = 8 << 10

// sensitiveByName reports whether the base name of file looks like a secret,
// from the name rules of ADR 0001 section 7. It ignores letter case, so a
// renamed copy such as ID_RSA still matches.
func sensitiveByName(file string) bool {
	name := strings.ToLower(path.Base(file))
	switch name {
	case "id_rsa", "id_ed25519", "id_ecdsa":
		return true
	}
	return strings.HasPrefix(name, ".env") || strings.HasSuffix(name, ".p12") || strings.HasSuffix(name, ".pfx")
}

// holdsPrivateKeyHeader reports whether a line of head has a -----BEGIN marker
// followed by PRIVATE KEY. It does not require the marker to start the line, so
// a key inside a JSON string still matches. Public keys and certificates do not.
func holdsPrivateKeyHeader(head []byte) bool {
	for _, line := range bytes.Split(head, []byte("\n")) {
		if _, afterMarker, found := bytes.Cut(line, []byte("-----BEGIN")); found && bytes.Contains(afterMarker, []byte("PRIVATE KEY")) {
			return true
		}
	}
	return false
}

// holdsPrivateKey reads the first bytes of file through root and reports
// whether they hold a private key header.
func holdsPrivateKey(root *os.Root, file string) (bool, error) {
	opened, err := root.Open(file)
	if err != nil {
		return false, fmt.Errorf("scan %q: %w", file, withoutPath(err))
	}
	defer opened.Close()

	head, err := io.ReadAll(io.LimitReader(opened, sensitiveContentWindow))
	if err != nil {
		return false, fmt.Errorf("scan %q: %w", file, withoutPath(err))
	}
	return holdsPrivateKeyHeader(head), nil
}

// checkSensitiveFiles fails with one error line for every file that looks like
// a secret by its name or its content, so the user can fix them in one pass.
// Each line shows the ignore rule that excludes the file. It quotes the path,
// because a file name can hold a newline that would forge an extra line. It
// stops at the first file it cannot read.
func checkSensitiveFiles(root *os.Root, files []string) error {
	var problems []error
	for _, file := range files {
		reason := ""
		if sensitiveByName(file) {
			reason = "the name looks like a secret"
		} else {
			holdsKey, err := holdsPrivateKey(root, file)
			if err != nil {
				return err
			}
			if holdsKey {
				reason = "the content holds a private key"
			}
		}
		if reason != "" {
			problems = append(problems, fmt.Errorf("scan %q: %s, add the ignore rule %q to exclude it", file, reason, file))
		}
	}
	return errors.Join(problems...)
}
