// Package config reads and validates the configuration files of Ship8t.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ship8t/ship8t/internal/strictjson"
)

const (
	// maxProjectFileBytes is the largest ship8t.json that DecodeProjectFile reads.
	maxProjectFileBytes = 64 << 10
	// maxDirectoryBytes is the longest directory setting in bytes.
	maxDirectoryBytes = 1024
	// maxProjectLength is the longest project slug, the length of a DNS label.
	maxProjectLength = 63
)

// ProjectFile holds the settings of a ship8t.json file. A setting that the file
// does not name keeps its zero value, so the caller can tell it apart and fall
// back to a lower-priority source.
type ProjectFile struct {
	// Project is the project slug. ADR 0003 defines its form.
	Project string
	// Directory is the directory to deploy, relative to the project directory.
	Directory string
	// Ignore and Include are patterns for the scanner. The scanner checks their
	// syntax.
	Ignore  []string
	Include []string
	Serve   Serve
}

// Serve holds the serving options of ship8t.json.
type Serve struct {
	SPAFallback bool
}

// DecodeProjectFile reads one ship8t.json from reader and validates it against
// ADR 0001, section 6. It reads at most 64 KiB, accepts only exact field names,
// and rejects a repeated field, an unknown field, a null value, a token, and
// data after the object. It never reads a file, so the caller controls where
// the bytes come from.
func DecodeProjectFile(reader io.Reader) (ProjectFile, error) {
	// Allow one byte more than the limit, so an over-long file is detectable.
	body, err := io.ReadAll(io.LimitReader(reader, maxProjectFileBytes+1))
	if err != nil {
		return ProjectFile{}, fmt.Errorf("read ship8t.json: %w", err)
	}
	if len(body) > maxProjectFileBytes {
		return ProjectFile{}, fmt.Errorf("invalid ship8t.json: larger than %d bytes", maxProjectFileBytes)
	}

	file, err := decodeProjectFields(body)
	if err != nil {
		return ProjectFile{}, fmt.Errorf("invalid ship8t.json: %w", err)
	}
	return file, nil
}

// decodeProjectFields decodes and validates the fields of the file body.
func decodeProjectFields(body []byte) (ProjectFile, error) {
	fields, err := strictjson.DecodeObject(body, "project", "directory", "ignore", "include", "serve")
	if err != nil {
		var unknown *strictjson.UnknownFieldError
		// Explain a token, because the file belongs in version control.
		if errors.As(err, &unknown) && strings.EqualFold(unknown.Name, "token") {
			return ProjectFile{}, errors.New("token is not allowed, because the file belongs in version control, set the SHIP8T_TOKEN environment variable instead")
		}
		return ProjectFile{}, err
	}

	var file ProjectFile
	if file.Project, err = decodeString(fields, "project", validateProject); err != nil {
		return ProjectFile{}, err
	}
	if file.Directory, err = decodeString(fields, "directory", validateDirectory); err != nil {
		return ProjectFile{}, err
	}
	if file.Ignore, err = decodeStringList(fields, "ignore"); err != nil {
		return ProjectFile{}, err
	}
	if file.Include, err = decodeStringList(fields, "include"); err != nil {
		return ProjectFile{}, err
	}
	if file.Serve, err = decodeServe(fields); err != nil {
		return ProjectFile{}, err
	}
	return file, nil
}

// decodeString decodes the field called name as a string and validates it. It
// returns the empty string when the object does not have the field.
func decodeString(fields map[string]json.RawMessage, name string, validate func(string) error) (string, error) {
	var value *string
	if err := decodeField(fields, name, &value); err != nil {
		return "", err
	}
	if value == nil {
		return "", nil
	}
	if err := validate(*value); err != nil {
		return "", err
	}
	return *value, nil
}

// decodeStringList decodes the field called name as a list of strings. It
// returns nil when the object does not have the field, and rejects a null entry,
// which the standard decoder would accept as an empty string.
func decodeStringList(fields map[string]json.RawMessage, name string) ([]string, error) {
	var entries []*string
	if err := decodeField(fields, name, &entries); err != nil {
		return nil, err
	}
	if _, found := fields[name]; !found {
		return nil, nil
	}
	list := make([]string, 0, len(entries))
	for index, entry := range entries {
		if entry == nil {
			return nil, fmt.Errorf("%s[%d] must be a string, not null", name, index)
		}
		list = append(list, *entry)
	}
	return list, nil
}

// decodeServe decodes the serve object, accepting only the exact field name
// spa_fallback.
func decodeServe(fields map[string]json.RawMessage) (Serve, error) {
	var serve Serve
	raw, found := fields["serve"]
	if !found {
		return serve, nil
	}
	if strictjson.IsNull(raw) {
		return serve, errors.New("serve must not be null, remove the field to use the default")
	}
	serveFields, err := strictjson.DecodeObject(raw, "spa_fallback")
	if err != nil {
		return serve, fmt.Errorf("serve: %w", err)
	}
	if err := decodeField(serveFields, "spa_fallback", &serve.SPAFallback); err != nil {
		return serve, fmt.Errorf("serve: %w", err)
	}
	return serve, nil
}

// validateProject reports why project is not a slug of ADR 0003, a DNS label of
// lowercase letters, digits, and hyphens that neither starts nor ends with a
// hyphen.
func validateProject(project string) error {
	// Check the length first, so an error never echoes an oversized value.
	if len(project) < 1 || len(project) > maxProjectLength {
		return fmt.Errorf("project must be 1 to %d characters, got %d", maxProjectLength, len(project))
	}
	for index := 0; index < len(project); index++ {
		character := project[index]
		isLowercaseLetter := character >= 'a' && character <= 'z'
		isDigit := character >= '0' && character <= '9'
		if !isLowercaseLetter && !isDigit && character != '-' {
			return fmt.Errorf("project %.64q must use only lowercase letters, digits, and hyphens", project)
		}
	}
	if project[0] == '-' || project[len(project)-1] == '-' {
		return fmt.Errorf("project %.64q must not start or end with a hyphen", project)
	}
	return nil
}

// validateDirectory reports why directory is not a clean relative path inside
// the project directory. The file can come from a repository that the user did
// not write, so a deploy must never read outside the project directory.
func validateDirectory(directory string) error {
	// Check the length first, so an error never echoes an oversized value.
	if len(directory) > maxDirectoryBytes {
		return fmt.Errorf("directory is %d bytes, over the limit of %d bytes", len(directory), maxDirectoryBytes)
	}
	switch {
	case directory == "":
		return errors.New("directory must not be empty, use . for the project directory")
	case !utf8.ValidString(directory) || strings.ContainsRune(directory, utf8.RuneError):
		return fmt.Errorf("directory %.64q must be valid UTF-8 without the replacement character", directory)
	case strings.ContainsFunc(directory, unicode.IsControl):
		return fmt.Errorf("directory %.64q must not contain a control character", directory)
	case strings.Contains(directory, `\`):
		return fmt.Errorf("directory %.64q must use / as the separator, not a backslash", directory)
	case strings.HasPrefix(directory, "/"):
		return fmt.Errorf("directory %.64q must be relative to the project directory, remove the leading /", directory)
	}
	for _, segment := range strings.Split(directory, "/") {
		if segment == ".." {
			return fmt.Errorf("directory %.64q must stay inside the project directory, remove the .. segment", directory)
		}
	}
	// ValidPath rejects the empty and . segments that the checks above allow.
	if !fs.ValidPath(directory) {
		return fmt.Errorf("directory %.64q must be a clean relative path, such as dist or build/public", directory)
	}
	return nil
}

// decodeField decodes the field called name into target when the object has it,
// and names the field in the error. Unlike the standard decoder, it rejects
// null, which would otherwise pass as an empty value.
func decodeField(fields map[string]json.RawMessage, name string, target any) error {
	if raw, found := fields[name]; found && strictjson.IsNull(raw) {
		return fmt.Errorf("%s must not be null, remove the field to use the default", name)
	}
	return strictjson.UnmarshalField(fields, name, target)
}
