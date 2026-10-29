package config

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ship8t/ship8t/internal/strictjson"
)

const (
	// maxUserFileBytes is the largest user config that DecodeUserFile reads.
	maxUserFileBytes = 16 << 10
	// maxServerBytes is the longest server URL in bytes.
	maxServerBytes = 2048
	// maxTokenBytes is the longest token in bytes.
	maxTokenBytes = 512
)

// UserFile holds the settings of the per-user config file. A setting that the
// file does not name keeps its zero value, so the caller can fall back to a
// lower-priority source.
type UserFile struct {
	// Server is the base URL of the Ship8t server.
	Server string
	// Token is the bearer token for the server. It is a secret.
	Token string
}

// DecodeUserFile reads one user config from reader and validates it against ADR
// 0001, section 6. It reads at most 16 KiB, accepts only exact field names, and
// rejects a repeated field, an unknown field, a null value, and data after the
// object. It never reads a file, and it never repeats the server or the token
// in an error, because either can hold a secret.
func DecodeUserFile(reader io.Reader) (UserFile, error) {
	// Allow one byte more than the limit, so an over-long file is detectable.
	body, err := io.ReadAll(io.LimitReader(reader, maxUserFileBytes+1))
	if err != nil {
		return UserFile{}, fmt.Errorf("read user config: %w", err)
	}
	if len(body) > maxUserFileBytes {
		return UserFile{}, fmt.Errorf("invalid user config: larger than %d bytes", maxUserFileBytes)
	}

	file, err := decodeUserFields(body)
	if err != nil {
		return UserFile{}, fmt.Errorf("invalid user config: %w", err)
	}
	return file, nil
}

// decodeUserFields decodes and validates the fields of the file body.
func decodeUserFields(body []byte) (UserFile, error) {
	fields, err := strictjson.DecodeObject(body, "server", "token")
	if err != nil {
		return UserFile{}, err
	}

	var file UserFile
	if file.Server, err = decodeString(fields, "server", validateServer); err != nil {
		return UserFile{}, err
	}
	if file.Token, err = decodeString(fields, "token", validateToken); err != nil {
		return UserFile{}, err
	}
	return file, nil
}

// validateServer reports why server is not the base URL of a Ship8t server, an
// http or https address without credentials, path, query, or fragment. It never
// repeats the value in an error, and it never wraps the parse error, because
// both can hold a password.
func validateServer(server string) error {
	// Check the length first, so a huge value is never parsed.
	if len(server) > maxServerBytes {
		return fmt.Errorf("server is %d bytes, over the limit of %d bytes", len(server), maxServerBytes)
	}
	if server == "" {
		return errors.New("server must not be empty, use an address such as https://deploy.example.com")
	}

	// The client prints the server in its results, and an invisible or direction
	// changing character can make one host look like another.
	for _, character := range server {
		if !unicode.IsPrint(character) || character == utf8.RuneError {
			return errors.New("server must use only printable characters, without invisible or control characters")
		}
	}

	parsed, err := url.Parse(server)
	if err != nil {
		return errors.New("server must be a valid address such as https://deploy.example.com")
	}
	if scheme := strings.ToLower(parsed.Scheme); scheme != "https" && scheme != "http" {
		return errors.New("server must start with https:// or http://")
	}
	if parsed.User != nil {
		return errors.New("server must not contain a user name or password, set the token in the token setting")
	}
	if parsed.Hostname() == "" {
		return errors.New("server must name a host")
	}
	if err := validatePort(parsed); err != nil {
		return err
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return errors.New("server must not have a path, use only the scheme, the host, and the port")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return errors.New("server must not have a query")
	}
	// The parser drops an empty fragment, so look at the text.
	if parsed.Fragment != "" || strings.Contains(server, "#") {
		return errors.New("server must not have a fragment")
	}
	return nil
}

// validatePort reports why the port of a parsed server address is not a port
// number. A server without a port is valid, and uses the port of its scheme.
func validatePort(parsed *url.URL) error {
	// The parser accepts a colon with no digits after it.
	if strings.HasSuffix(parsed.Host, ":") {
		return errors.New("server has a colon but no port, remove the colon or add a port from 1 to 65535")
	}
	if parsed.Port() == "" {
		return nil
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		return errors.New("server port must be from 1 to 65535")
	}
	return nil
}

// validateToken reports why token cannot be sent as a bearer token. The token
// goes into an HTTP header, so it must be visible ASCII, with no space and no
// control character that could add a header line. It never repeats the token.
func validateToken(token string) error {
	if len(token) < 1 || len(token) > maxTokenBytes {
		return fmt.Errorf("token must be 1 to %d characters, got %d", maxTokenBytes, len(token))
	}
	for index := 0; index < len(token); index++ {
		if character := token[index]; character <= ' ' || character >= 0x7f {
			return errors.New("token must use only visible ASCII characters, with no spaces or control characters")
		}
	}
	return nil
}
