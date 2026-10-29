package config

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestDecodeUserFileRejectsInvalidFiles(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantInErr []string
	}{
		// Field names.
		{"unknown field", `{"project":"demo"}`, []string{"unknown field", "project"}},
		{"field with the wrong case", `{"Server":"https://example.com"}`, []string{"unknown field", "case sensitive"}},
		{"field with the wrong case for the token", `{"TOKEN":"abc"}`, []string{"unknown field"}},
		{"repeated server", `{"server":"https://a.example","server":"https://b.example"}`, []string{"appears more than once", "server"}},
		{"repeated token", `{"token":"abc","token":"def"}`, []string{"appears more than once", "token"}},

		// Shape of the file.
		{"empty input", ``, []string{"invalid user config"}},
		{"truncated object", `{"server":"https://example.com"`, []string{"invalid user config"}},
		{"array", `[]`, []string{"JSON object"}},
		{"null", `null`, []string{"JSON object"}},
		{"second object", `{} {}`, []string{"unexpected data"}},
		{"text after the object", `{} trailing`, []string{"unexpected data"}},

		// Types and null.
		{"server that is a number", `{"server":1}`, []string{"server"}},
		{"server that is null", `{"server":null}`, []string{"server", "null"}},
		{"token that is a number", `{"token":1}`, []string{"token"}},
		{"token that is null", `{"token":null}`, []string{"token", "null"}},
		{"token that is a list", `{"token":["abc"]}`, []string{"token"}},

		// server must be a plain base URL of the Ship8t server.
		{"empty server", `{"server":""}`, []string{"server", "empty"}},
		{"server without a scheme", `{"server":"example.com"}`, []string{"server", "https://"}},
		{"server with only a colon and a port", `{"server":"example.com:8481"}`, []string{"server"}},
		{"server with a protocol relative address", `{"server":"//example.com"}`, []string{"server", "https://"}},
		{"server with the ftp scheme", `{"server":"ftp://example.com"}`, []string{"server", "https://"}},
		{"server with the file scheme", `{"server":"file:///etc/passwd"}`, []string{"server", "https://"}},
		{"server with the javascript scheme", `{"server":"javascript:alert(1)"}`, []string{"server", "https://"}},
		{"server with an opaque address", `{"server":"https:example.com"}`, []string{"server"}},
		{"server without a host", `{"server":"https://"}`, []string{"server", "host"}},
		{"server with only a port", `{"server":"https://:8481"}`, []string{"server", "host"}},
		{"server with a space in the host", `{"server":"https://exa mple.com"}`, []string{"server"}},
		{"server with a leading space", `{"server":" https://example.com"}`, []string{"server"}},
		{"server with a trailing newline", `{"server":"https://example.com\n"}`, []string{"server"}},
		{"server with a carriage return", `{"server":"https://example.com\r\nHost: evil"}`, []string{"server"}},
		{"server with a backslash", `{"server":"https://example.com\\evil"}`, []string{"server"}},
		{"server with an empty port", `{"server":"https://example.com:"}`, []string{"server", "port"}},
		{"server with port zero", `{"server":"https://example.com:0"}`, []string{"server", "port"}},
		{"server with a port over 65535", `{"server":"https://example.com:65536"}`, []string{"server", "port"}},
		{"server with a path", `{"server":"https://example.com/v1"}`, []string{"server", "path"}},
		{"server with a deep path", `{"server":"https://example.com/a/b"}`, []string{"server", "path"}},
		{"server with a query", `{"server":"https://example.com?x=1"}`, []string{"server", "query"}},
		{"server with an empty query", `{"server":"https://example.com/?"}`, []string{"server", "query"}},
		{"server with a fragment", `{"server":"https://example.com/#top"}`, []string{"server", "fragment"}},
		{"server with an empty fragment", `{"server":"https://example.com#"}`, []string{"server", "fragment"}},
		{"server over 2048 bytes", `{"server":"https://` + strings.Repeat("a", 2048) + `.example"}`, []string{"server", "2048"}},

		// Invisible and direction-changing characters can make one host look like
		// another when the client prints the server.
		{"server with a line separator", `{"server":"https://exa\u2028mple.com"}`, []string{"server", "printable"}},
		{"server with a next line character", `{"server":"https://exa\u0085mple.com"}`, []string{"server", "printable"}},
		{"server with a zero width space", `{"server":"https://exa\u200bmple.com"}`, []string{"server", "printable"}},
		{"server with a no-break space", `{"server":"https://exa\u00a0mple.com"}`, []string{"server", "printable"}},
		{"server with a right-to-left mark", `{"server":"https://exa\u200fmple.com"}`, []string{"server", "printable"}},
		{"server with a right-to-left override", `{"server":"https://exa\u202emple.com"}`, []string{"server", "printable"}},
		{"server with a private use character", `{"server":"https://exa\ue000mple.com"}`, []string{"server", "printable"}},
		{"server with the replacement character", `{"server":"https://exa\ufffdmple.com"}`, []string{"server", "printable"}},

		// A server with credentials would put a secret in a file and in logs.
		{"server with a user name and password", `{"server":"https://alice:hunter2@example.com"}`, []string{"server", "user name or password"}},
		{"server with only a user name", `{"server":"https://alice@example.com"}`, []string{"server", "user name or password"}},
		{"server with an empty user", `{"server":"https://@example.com"}`, []string{"server", "user name or password"}},
		{"server that hides a host behind credentials", `{"server":"https://example.com:443@evil.example"}`, []string{"server", "user name or password"}},

		// The token goes into a header, so it must be visible ASCII with no spaces.
		{"empty token", `{"token":""}`, []string{"token", "1 to 512"}},
		{"token over 512 bytes", `{"token":"` + strings.Repeat("a", 513) + `"}`, []string{"token", "1 to 512"}},
		{"token with a space", `{"token":"abc def"}`, []string{"token", "visible ASCII"}},
		{"token with a leading space", `{"token":" abc"}`, []string{"token", "visible ASCII"}},
		{"token with a tab", `{"token":"abc\tdef"}`, []string{"token", "visible ASCII"}},
		{"token with a newline", `{"token":"abc\ndef"}`, []string{"token", "visible ASCII"}},
		{"token that injects a header", `{"token":"abc\r\nX-Evil: 1"}`, []string{"token", "visible ASCII"}},
		{"token with a null character", `{"token":"abc\u0000def"}`, []string{"token", "visible ASCII"}},
		{"token with the delete character", `{"token":"abc\u007fdef"}`, []string{"token", "visible ASCII"}},
		{"token with a non-ASCII letter", `{"token":"abcé"}`, []string{"token", "visible ASCII"}},
		{"token with the replacement character", `{"token":"abc�"}`, []string{"token", "visible ASCII"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := DecodeUserFile(strings.NewReader(test.body))
			if err == nil {
				t.Fatalf("DecodeUserFile(%s) = %+v, nil error, want an error", test.body, got)
			}
			if !reflect.DeepEqual(got, UserFile{}) {
				t.Errorf("DecodeUserFile(%s) = %+v with an error, want the zero value", test.body, got)
			}
			for _, want := range test.wantInErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("DecodeUserFile(%s) error = %q, want it to contain %q", test.body, err, want)
				}
			}
		})
	}
}

func TestDecodeUserFileNeverEchoesASecret(t *testing.T) {
	// Every value below holds a word that must not reach an error message. The
	// message ends up in a terminal, in a log, or in a bug report.
	tests := []struct {
		name   string
		body   string
		secret string
	}{
		{"token with a space", `{"token":"s3cret value"}`, "s3cret"},
		{"token that is too long", `{"token":"s3cret` + strings.Repeat("a", 600) + `"}`, "s3cret"},
		{"token with a control character", `{"token":"s3cret\u0001"}`, "s3cret"},
		{"token that is a number", `{"token":12345678}`, "12345678"},
		{"server with a password", `{"server":"https://alice:hunter2@example.com"}`, "hunter2"},
		{"server with a password and a bad scheme", `{"server":"ftp://alice:hunter2@example.com"}`, "hunter2"},
		{"server with a password and a bad port", `{"server":"https://alice:hunter2@example.com:99999"}`, "hunter2"},
		{"server that does not parse", `{"server":"https://alice:hunter2@exa mple.com"}`, "hunter2"},
		{"server with a password and a path", `{"server":"https://alice:hunter2@example.com/v1"}`, "hunter2"},
		{"server with a secret in the query", `{"server":"https://example.com/?key=s3cret"}`, "s3cret"},
		{"server with a secret in the path", `{"server":"https://example.com/s3cret"}`, "s3cret"},
		{"server with a secret in the fragment", `{"server":"https://example.com/#s3cret"}`, "s3cret"},
		{"server with a secret in an unknown scheme", `{"server":"s3cret://example.com"}`, "s3cret"},
		{"server with a secret and no scheme", `{"server":"s3cret"}`, "s3cret"},
		{"unknown field that holds a secret value", `{"password":"s3cret"}`, "s3cret"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeUserFile(strings.NewReader(test.body))
			if err == nil {
				t.Fatalf("DecodeUserFile(%s) = nil error, want an error", test.body)
			}
			if strings.Contains(err.Error(), test.secret) {
				t.Errorf("DecodeUserFile(%s) error = %q, want it not to repeat %q", test.body, err, test.secret)
			}
		})
	}
}

func TestDecodeUserFileAcceptsValidFiles(t *testing.T) {
	tests := []struct {
		name string
		body string
		want UserFile
	}{
		{"empty object", `{}`, UserFile{}},
		{"surrounding whitespace", " \n\t{ }\n ", UserFile{}},
		{
			name: "server and token",
			body: `{"server":"https://deploy.example.com","token":"abc123"}`,
			want: UserFile{Server: "https://deploy.example.com", Token: "abc123"},
		},
		{"settings in any order", `{"token":"abc","server":"https://a.example"}`, UserFile{Server: "https://a.example", Token: "abc"}},
		{"server only", `{"server":"https://deploy.example.com"}`, UserFile{Server: "https://deploy.example.com"}},
		{"token only", `{"token":"abc123"}`, UserFile{Token: "abc123"}},
		{"http server on a loopback address", `{"server":"http://127.0.0.1:8481"}`, UserFile{Server: "http://127.0.0.1:8481"}},
		{"http server on localhost", `{"server":"http://localhost:8481"}`, UserFile{Server: "http://localhost:8481"}},
		{"http server on an IPv6 loopback address", `{"server":"http://[::1]:8481"}`, UserFile{Server: "http://[::1]:8481"}},
		{"server with a trailing slash", `{"server":"https://deploy.example.com/"}`, UserFile{Server: "https://deploy.example.com/"}},
		{"server with a project hostname", `{"server":"http://demo.localhost:8480"}`, UserFile{Server: "http://demo.localhost:8480"}},
		{"server with capitals in the scheme", `{"server":"HTTPS://deploy.example.com"}`, UserFile{Server: "HTTPS://deploy.example.com"}},
		{"server on the highest port", `{"server":"https://example.com:65535"}`, UserFile{Server: "https://example.com:65535"}},
		{"server on the lowest port", `{"server":"https://example.com:1"}`, UserFile{Server: "https://example.com:1"}},
		{"server with a non-ASCII host", `{"server":"https://münchen.example"}`, UserFile{Server: "https://münchen.example"}},
		{"token of one character", `{"token":"a"}`, UserFile{Token: "a"}},
		{"token of 512 characters", `{"token":"` + strings.Repeat("a", 512) + `"}`, UserFile{Token: strings.Repeat("a", 512)}},
		{"token with punctuation", `{"token":"a-b_c.d~e+f/g=h!i#j$k%l&m'n*o"}`, UserFile{Token: "a-b_c.d~e+f/g=h!i#j$k%l&m'n*o"}},
		{"token at the edges of visible ASCII", `{"token":"!~"}`, UserFile{Token: "!~"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := DecodeUserFile(strings.NewReader(test.body))
			if err != nil {
				t.Fatalf("DecodeUserFile(%s) error = %v", test.body, err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("DecodeUserFile(%s) = %+v, want %+v", test.body, got, test.want)
			}
		})
	}
}

func TestDecodeUserFileSizeLimit(t *testing.T) {
	// padded returns a valid file of exactly size bytes, with spaces after the
	// object.
	padded := func(size int) string {
		body := `{"token":"abc"}`
		return body + strings.Repeat(" ", size-len(body))
	}

	if _, err := DecodeUserFile(strings.NewReader(padded(maxUserFileBytes))); err != nil {
		t.Errorf("DecodeUserFile() of exactly %d bytes error = %v, want nil", maxUserFileBytes, err)
	}

	_, err := DecodeUserFile(strings.NewReader(padded(maxUserFileBytes + 1)))
	if err == nil || !strings.Contains(err.Error(), "larger than 16384 bytes") {
		t.Errorf("DecodeUserFile() of %d bytes error = %v, want it to name the limit of 16384 bytes", maxUserFileBytes+1, err)
	}
}

func TestDecodeUserFileStopsReadingAnEndlessInput(t *testing.T) {
	reader := &endlessReader{}

	if _, err := DecodeUserFile(reader); err == nil {
		t.Fatal("DecodeUserFile() of an endless input = nil error, want an error")
	}
	if reader.given > maxUserFileBytes+1 {
		t.Errorf("DecodeUserFile() read %d bytes, want at most %d", reader.given, maxUserFileBytes+1)
	}
}

func TestDecodeUserFileCapsErrorsThatQuoteInput(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"unknown field name", `{"` + strings.Repeat("x", 15000) + `":1}`},
		{"server value", `{"server":"` + strings.Repeat("A", 15000) + `"}`},
		{"token value", `{"token":"` + strings.Repeat("a", 15000) + `"}`},
		{"type mismatch", `{"token":` + strings.Repeat("9", 15000) + `}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeUserFile(strings.NewReader(test.body))
			if err == nil {
				t.Fatal("DecodeUserFile() = nil error, want an error")
			}
			if len(err.Error()) > 400 {
				t.Errorf("DecodeUserFile() error is %d bytes, want it capped near 400", len(err.Error()))
			}
		})
	}
}

func TestDecodeUserFileLimitsAreExact(t *testing.T) {
	// serverOfSize returns a server address of exactly size bytes.
	serverOfSize := func(size int) string {
		const prefix = "https://"
		return prefix + strings.Repeat("a", size-len(prefix))
	}

	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"server of exactly 2048 bytes", `{"server":"` + serverOfSize(maxServerBytes) + `"}`, false},
		{"server of 2049 bytes", `{"server":"` + serverOfSize(maxServerBytes+1) + `"}`, true},
		{"token of exactly 512 bytes", `{"token":"` + strings.Repeat("a", maxTokenBytes) + `"}`, false},
		{"token of 513 bytes", `{"token":"` + strings.Repeat("a", maxTokenBytes+1) + `"}`, true},
		{"token of exactly one byte", `{"token":"a"}`, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeUserFile(strings.NewReader(test.body))
			if (err != nil) != test.wantErr {
				t.Errorf("DecodeUserFile() error = %v, want an error: %v", err, test.wantErr)
			}
		})
	}
}

// failingReader fails at the first read, as a file that cannot be read does.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("disk failure")
}

func TestDecodeUserFileReportsAReadError(t *testing.T) {
	_, err := DecodeUserFile(failingReader{})
	if err == nil {
		t.Fatal("DecodeUserFile() = nil error, want an error")
	}
	for _, want := range []string{"read user config", "disk failure"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("DecodeUserFile() error = %q, want it to contain %q", err, want)
		}
	}
}

// canary is a value that no error message holds by chance. A test puts it where a
// secret would be, and then looks for any part of it in the error.
const canary = "Zq7Xk2Wm9"

// canaryPart returns the first piece of four bytes of the canary that text
// holds, or the empty string when it holds none.
func canaryPart(text string) string {
	for start := 0; start+4 <= len(canary); start++ {
		if piece := canary[start : start+4]; strings.Contains(text, piece) {
			return piece
		}
	}
	return ""
}

func TestDecodeUserFileNeverEchoesAnyPartOfAValue(t *testing.T) {
	// CANARY stands where a secret could be. A message that cuts the first few
	// characters of a value, or repeats only its host, would still show a piece.
	templates := []string{
		`{"server":"https://alice:CANARY@example.com"}`,
		`{"server":"ftp://alice:CANARY@example.com"}`,
		`{"server":"https://alice:CANARY@example.com:99999"}`,
		`{"server":"https://alice:CANARY@exa mple.com"}`,
		`{"server":"https://alice:CANARY@example.com/v1"}`,
		`{"server":"https://CANARY.example/CANARY"}`,
		`{"server":"https://CANARY.example?key=CANARY"}`,
		`{"server":"https://CANARY.example/#CANARY"}`,
		`{"server":"https://CANARY.example:"}`,
		`{"server":"https://CANARY.example:0"}`,
		`{"server":"https://CANARY\t.example"}`,
		`{"server":"https://:8481/CANARY"}`,
		`{"server":"https://:CANARY"}`,
		`{"server":"https:CANARY"}`,
		`{"server":"https://CANARY.example:8481/CANARY?x=CANARY#CANARY"}`,
		`{"server":"CANARY"}`,
		`{"server":"CANARY://example.com"}`,
		`{"server":["CANARY"]}`,
		`{"server":{"key":"CANARY"}}`,
		`{"server":"https://` + strings.Repeat("a", 2048) + `CANARY"}`,
		`{"token":"CANARY CANARY"}`,
		`{"token":"CANARY\tx"}`,
		`{"token":"CANARYé"}`,
		`{"token":["CANARY"]}`,
		`{"token":{"key":"CANARY"}}`,
		`{"token":"` + strings.Repeat("a", 600) + `CANARY"}`,
		`{"server":"https://CANARY.example","token":"CANARY CANARY"}`,
		`{"server":"ftp://CANARY.example","token":"CANARY"}`,
		`{"token":"CANARY","token":"CANARY"}`,
		`{"server":"CANARY","server":"CANARY"}`,
	}

	for index, template := range templates {
		body := strings.ReplaceAll(template, "CANARY", canary)
		t.Run(strings.Join([]string{"case", string(rune('a' + index))}, " "), func(t *testing.T) {
			_, err := DecodeUserFile(strings.NewReader(body))
			if err == nil {
				t.Fatalf("DecodeUserFile(%.80s) = nil error, want an error", body)
			}
			if piece := canaryPart(err.Error()); piece != "" {
				t.Errorf("DecodeUserFile(%.80s) error = %q, want it not to repeat any part of the value, but it holds %q", body, err, piece)
			}
		})
	}
}
