package scan

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

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

// checkSensitiveFiles fails with one error line for every file that looks like
// a secret, so the user can fix them in one pass. Each line shows the ignore
// rule that excludes the file. It quotes the path, because a file name can hold
// a newline that would forge an extra line.
func checkSensitiveFiles(files []string) error {
	var problems []error
	for _, file := range files {
		if sensitiveByName(file) {
			problems = append(problems, fmt.Errorf("scan %q: the name looks like a secret, add the ignore rule %q to exclude it", file, file))
		}
	}
	return errors.Join(problems...)
}
