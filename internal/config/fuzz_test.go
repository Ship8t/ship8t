package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/ship8t/ship8t/internal/strictjson"
)

// projectSeeds returns a few valid files and one input for each rule of the
// project file, so the fuzzer starts from both sides of every rule.
func projectSeeds() []string {
	return []string{
		`{}`,
		`{"project":"my-site","directory":"dist","ignore":["**/*.map","drafts/"],"include":[".well-known/**"],"serve":{"spa_fallback":true}}`,
		`{"project":"a"}`,
		`{"directory":"."}`,
		`{"directory":"build/public"}`,
		`{"ignore":[],"include":[]}`,
		`{"ignore":["a\tb"]}`,
		`{"ignore":["` + string([]byte{0xff, 0xfe}) + `"]}`,
		`{"serve":{"spa_fallback":false}}`,
		// Field rules.
		`{"token":"s3cret"}`,
		`{"TOKEN":"s3cret"}`,
		`{"server":"https://example.com"}`,
		`{"Project":"a"}`,
		`{"project":"a","project":"b"}`,
		`{"serve":{"mode":"spa"}}`,
		`{"serve":{"SPA_fallback":true}}`,
		`{"project":null}`,
		`{"directory":null}`,
		`{"ignore":null}`,
		`{"ignore":["a",null]}`,
		`{"serve":null}`,
		`{"serve":{"spa_fallback":null}}`,
		`{"project":1}`,
		`{"ignore":"*.map"}`,
		`{"serve":{"spa_fallback":"yes"}}`,
		// Project slugs.
		`{"project":""}`,
		`{"project":"My-Site"}`,
		`{"project":"-site"}`,
		`{"project":"site-"}`,
		`{"project":"my_site"}`,
		`{"project":"café"}`,
		`{"project":"` + strings.Repeat("a", 64) + `"}`,
		// Directories.
		`{"directory":""}`,
		`{"directory":"/etc"}`,
		`{"directory":".."}`,
		`{"directory":"../dist"}`,
		`{"directory":"a/../b"}`,
		`{"directory":"./dist"}`,
		`{"directory":"a//b"}`,
		`{"directory":"dist/"}`,
		`{"directory":"a\\b"}`,
		`{"directory":"a\u0000b"}`,
		`{"directory":"a\ufffdb"}`,
		`{"directory":"` + strings.Repeat("a", 1025) + `"}`,
		// Shape.
		`{} {}`,
		`{} trailing`,
		`[]`,
		`null`,
		``,
		`{`,
		`{"project":`,
	}
}

// isSlug reports whether project is a DNS label slug of ADR 0003. The check is
// written apart from the decoder.
func isSlug(project string) bool {
	if len(project) < 1 || len(project) > 63 {
		return false
	}
	for index := 0; index < len(project); index++ {
		character := project[index]
		if !(character >= 'a' && character <= 'z') && !(character >= '0' && character <= '9') && character != '-' {
			return false
		}
	}
	return project[0] != '-' && project[len(project)-1] != '-'
}

// checkAcceptedDirectory fails the test unless an accepted directory is a clean
// relative path that stays inside the project directory. It checks the text and
// then asks the path package where the directory leads, so a rule that the text
// check misses still shows.
func checkAcceptedDirectory(t *testing.T, directory string) {
	t.Helper()
	if directory == "" {
		return
	}
	if len(directory) > 1024 {
		t.Errorf("accepted a directory of %d bytes, over the limit of 1024", len(directory))
	}
	if !utf8.ValidString(directory) || strings.ContainsRune(directory, utf8.RuneError) {
		t.Errorf("accepted the directory %q that is not clean UTF-8", directory)
	}
	if strings.ContainsFunc(directory, unicode.IsControl) || strings.Contains(directory, `\`) {
		t.Errorf("accepted the directory %q with a control character or a backslash", directory)
	}
	if strings.HasPrefix(directory, "/") {
		t.Errorf("accepted the absolute directory %q", directory)
	}
	if directory != "." {
		for _, segment := range strings.Split(directory, "/") {
			if segment == "" || segment == "." || segment == ".." {
				t.Errorf("accepted the directory %q with the segment %q", directory, segment)
			}
		}
	}

	const base = "/base/project"
	relative, err := filepath.Rel(base, filepath.Join(base, filepath.FromSlash(directory)))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Errorf("accepted the directory %q that leads out of the project directory to %q, error %v", directory, relative, err)
	}
}

// encodeJSON writes object as JSON without escaping HTML, so the encoding is
// not longer than the text that the decoder read.
func encodeJSON(t *testing.T, object map[string]any) []byte {
	t.Helper()
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(object); err != nil {
		t.Fatalf("encode %v: %v", object, err)
	}
	return buffer.Bytes()
}

// encodeProjectFile writes the settings of file as the project file would hold
// them. It leaves out a setting that is empty, because the decoder rejects an
// empty project or directory, and it keeps an empty list apart from no list.
func encodeProjectFile(t *testing.T, file ProjectFile) []byte {
	t.Helper()
	object := map[string]any{"serve": map[string]any{"spa_fallback": file.Serve.SPAFallback}}
	if file.Project != "" {
		object["project"] = file.Project
	}
	if file.Directory != "" {
		object["directory"] = file.Directory
	}
	if file.Ignore != nil {
		object["ignore"] = file.Ignore
	}
	if file.Include != nil {
		object["include"] = file.Include
	}
	return encodeJSON(t, object)
}

// jsonEntry is one field of a JSON object, in the order of the input.
type jsonEntry struct {
	name string
	raw  json.RawMessage
}

// readObject reads data as exactly one JSON object with nothing after it, and
// returns its fields in order. It does not use the decoders under test, so it is
// a second opinion about what the input says. It reports false for anything
// else.
func readObject(data []byte) ([]jsonEntry, bool) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil, false
	}
	var entries []jsonEntry
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, false
		}
		name, isString := token.(string)
		if !isString {
			return nil, false
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, false
		}
		entries = append(entries, jsonEntry{name: name, raw: raw})
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, false
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, false
	}
	return entries, true
}

// acceptedFields fails the test unless an accepted input holds only the allowed
// field names, each once and none of them null, and returns the raw value of
// each field. The decoders must reject an input that breaks these rules, so an
// accepted input that does shows a decoder that is too lenient.
func acceptedFields(t *testing.T, data []byte, allowed ...string) map[string]json.RawMessage {
	t.Helper()
	entries, ok := readObject(data)
	if !ok {
		t.Fatalf("accepted %q, which is not exactly one JSON object", data)
	}
	fields := make(map[string]json.RawMessage, len(entries))
	for _, entry := range entries {
		if !slices.Contains(allowed, entry.name) {
			t.Errorf("accepted the field %q, which is not one of %v", entry.name, allowed)
		}
		if _, repeated := fields[entry.name]; repeated {
			t.Errorf("accepted the field %q more than once", entry.name)
		}
		if strictjson.IsNull(entry.raw) {
			t.Errorf("accepted null for the field %q", entry.name)
		}
		fields[entry.name] = entry.raw
	}
	return fields
}

// wantString returns the string that the input holds for a field, or the empty
// string when the field is absent. An accepted input never holds an empty
// string, so it reports one.
func wantString(t *testing.T, fields map[string]json.RawMessage, name string) string {
	t.Helper()
	raw, found := fields[name]
	if !found {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Errorf("accepted %s that is not a string: %s", name, raw)
		return ""
	}
	if value == "" {
		t.Errorf("accepted an empty %s", name)
	}
	return value
}

// wantStringList returns the strings that the input holds for a list field, nil
// when the field is absent, and an empty list for an empty array.
func wantStringList(t *testing.T, fields map[string]json.RawMessage, name string) []string {
	t.Helper()
	raw, found := fields[name]
	if !found {
		return nil
	}
	var entries []*string
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Errorf("accepted %s that is not a list of strings: %s", name, raw)
		return nil
	}
	list := make([]string, 0, len(entries))
	for index, entry := range entries {
		if entry == nil {
			t.Errorf("accepted null as entry %d of %s", index, name)
			continue
		}
		list = append(list, *entry)
	}
	return list
}

// checkAcceptedProjectAgainstInput fails the test unless the settings of an
// accepted project file are exactly what the input says. A decoder that trims,
// lowercases, truncates, drops a field, or keeps the last of a repeated field
// would return valid settings, so the checks of the values alone cannot see it.
func checkAcceptedProjectAgainstInput(t *testing.T, body []byte, file ProjectFile) {
	t.Helper()
	fields := acceptedFields(t, body, "project", "directory", "ignore", "include", "serve")
	if want := wantString(t, fields, "project"); file.Project != want {
		t.Errorf("project = %q, want exactly what the input says, %q", file.Project, want)
	}
	if want := wantString(t, fields, "directory"); file.Directory != want {
		t.Errorf("directory = %q, want exactly what the input says, %q", file.Directory, want)
	}
	if want := wantStringList(t, fields, "ignore"); !reflect.DeepEqual(file.Ignore, want) {
		t.Errorf("ignore = %q, want exactly what the input says, %q", file.Ignore, want)
	}
	if want := wantStringList(t, fields, "include"); !reflect.DeepEqual(file.Include, want) {
		t.Errorf("include = %q, want exactly what the input says, %q", file.Include, want)
	}

	wantSPAFallback := false
	if raw, found := fields["serve"]; found {
		serveFields := acceptedFields(t, raw, "spa_fallback")
		if value, found := serveFields["spa_fallback"]; found {
			if err := json.Unmarshal(value, &wantSPAFallback); err != nil {
				t.Errorf("accepted spa_fallback that is not a boolean: %s", value)
			}
		}
	}
	if file.Serve.SPAFallback != wantSPAFallback {
		t.Errorf("spa_fallback = %v, want exactly what the input says, %v", file.Serve.SPAFallback, wantSPAFallback)
	}
}

// checkAcceptedUserAgainstInput fails the test unless the settings of an
// accepted user config are exactly what the input says.
func checkAcceptedUserAgainstInput(t *testing.T, body []byte, file UserFile) {
	t.Helper()
	fields := acceptedFields(t, body, "server", "token")
	if want := wantString(t, fields, "server"); file.Server != want {
		t.Errorf("server = %q, want exactly what the input says, %q", file.Server, want)
	}
	if want := wantString(t, fields, "token"); file.Token != want {
		t.Errorf("token = %q, want exactly what the input says, %q", file.Token, want)
	}
}

func FuzzDecodeProjectFile(f *testing.F) {
	for _, seed := range projectSeeds() {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, body []byte) {
		file, err := DecodeProjectFile(bytes.NewReader(body))
		if err != nil {
			if !reflect.DeepEqual(file, ProjectFile{}) {
				t.Fatalf("DecodeProjectFile() returned %+v and the error %v", file, err)
			}
			return
		}

		if len(body) > maxProjectFileBytes {
			t.Errorf("accepted a file of %d bytes, over the limit of %d", len(body), maxProjectFileBytes)
		}
		if file.Project != "" && !isSlug(file.Project) {
			t.Errorf("accepted the project %q that is not a slug", file.Project)
		}
		checkAcceptedDirectory(t, file.Directory)
		checkAcceptedProjectAgainstInput(t, body, file)

		// Decode what a tool would write for these settings. It must give the same
		// settings, or a saved file could change meaning.
		encoded := encodeProjectFile(t, file)
		if len(encoded) > maxProjectFileBytes {
			// Invalid UTF-8 in a pattern becomes three bytes in the encoding, so a
			// file near the limit can grow past it.
			return
		}
		again, err := DecodeProjectFile(bytes.NewReader(encoded))
		if err != nil {
			t.Fatalf("DecodeProjectFile() rejected the encoding of accepted settings %s: %v", encoded, err)
		}
		if !reflect.DeepEqual(file, again) {
			t.Errorf("DecodeProjectFile() of the encoding = %+v, want %+v", again, file)
		}
	})
}

// userSeeds returns a few valid files and one input for each rule of the user
// config, so the fuzzer starts from both sides of every rule.
func userSeeds() []string {
	return []string{
		`{}`,
		`{"server":"https://deploy.example.com","token":"abc123"}`,
		`{"server":"http://127.0.0.1:8481"}`,
		`{"server":"http://[::1]:8481/"}`,
		`{"server":"https://münchen.example"}`,
		`{"server":"HTTPS://deploy.example.com:65535"}`,
		`{"token":"a-b_c.d~e+f/g=h!i#j$k%l&m'n*o"}`,
		// Field rules.
		`{"project":"demo"}`,
		`{"Server":"https://example.com"}`,
		`{"server":"https://a.example","server":"https://b.example"}`,
		`{"server":null}`,
		`{"token":null}`,
		`{"server":1}`,
		`{"token":["abc"]}`,
		// Server rules.
		`{"server":""}`,
		`{"server":"example.com"}`,
		`{"server":"ftp://example.com"}`,
		`{"server":"file:///etc/passwd"}`,
		`{"server":"javascript:alert(1)"}`,
		`{"server":"https://"}`,
		`{"server":"https://:8481"}`,
		`{"server":"https://exa mple.com"}`,
		`{"server":"https://example.com\r\nHost: evil"}`,
		`{"server":"https://example.com:"}`,
		`{"server":"https://example.com:0"}`,
		`{"server":"https://example.com:65536"}`,
		`{"server":"https://example.com/v1"}`,
		`{"server":"https://example.com?x=1"}`,
		`{"server":"https://example.com#top"}`,
		`{"server":"https://alice:hunter2@example.com"}`,
		`{"server":"https://alice:hunter2@exa mple.com"}`,
		`{"server":"https://example.com:443@evil.example"}`,
		`{"server":"https://` + strings.Repeat("a", 2048) + `.example"}`,
		`{"server":" https://example.com"}`,
		`{"server":"https://deploy.example.com/"}`,
		`{"server":"https://exa\u202emple.com"}`,
		`{"server":"https://exa\u200bmple.com"}`,
		`{"server":"https://exa\u2028mple.com"}`,
		`{"server":"https://exa\ufffdmple.com"}`,
		// Token rules.
		`{"token":""}`,
		`{"token":"abc def"}`,
		`{"token":"s3cret value"}`,
		`{"token":" abc"}`,
		`{"token":"abc ","server":"https://example.com"}`,
		`{"token":"abc","token":"def"}`,
		`{"token":"abc\r\nX-Evil:1"}`,
		`{"token":"abc\nX-Evil:1"}`,
		`{"token":"abc\rX-Evil:1"}`,
		`{"token":"abc\r\nX-Evil: 1"}`,
		`{"token":"abc\u007fdef"}`,
		`{"token":"abcé"}`,
		`{"token":"` + strings.Repeat("a", 513) + `"}`,
		// Shape.
		`{} {}`,
		`[]`,
		`null`,
		``,
		`{`,
	}
}

// errNoDial marks the dial of the transport below, which never opens a
// connection.
var errNoDial = errors.New("the test does not dial")

// goRefusesRequest reports why the Go HTTP transport refuses a request to server
// with token as its bearer token, or nil when it gets as far as dialing. The
// transport checks the host and the header value before it dials, and the dialer
// here always fails, so the test never uses the network.
func goRefusesRequest(server, token string) error {
	request, err := http.NewRequest(http.MethodGet, server, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	transport := &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
		return nil, errNoDial
	}}
	_, err = transport.RoundTrip(request)
	if err != nil && !strings.Contains(err.Error(), errNoDial.Error()) {
		return err
	}
	return nil
}

// checkAcceptedUser fails the test unless an accepted user config holds the
// rules of ADR 0001 and can be used for a request. The text checks are written
// apart from the decoder. The Go HTTP transport is a second opinion, because it
// refuses a request with an invalid host or an invalid header value.
func checkAcceptedUser(t *testing.T, file UserFile) {
	t.Helper()
	server := file.Server
	if server != "" {
		if len(server) > 2048 {
			t.Errorf("accepted a server of %d bytes, over the limit of 2048", len(server))
		}
		for _, character := range server {
			if !unicode.IsPrint(character) || character == utf8.RuneError {
				t.Errorf("accepted the server %q with the unprintable character %q", server, character)
			}
		}
		if strings.ContainsAny(server, "?#@") {
			t.Errorf("accepted the server %q with a query, a fragment, or credentials", server)
		}

		parsed, err := url.Parse(server)
		if err != nil {
			t.Fatalf("accepted the server %q that does not parse: %v", server, err)
		}
		if scheme := strings.ToLower(parsed.Scheme); scheme != "http" && scheme != "https" {
			t.Errorf("accepted the server %q with the scheme %q", server, parsed.Scheme)
		}
		if parsed.Hostname() == "" || parsed.User != nil {
			t.Errorf("accepted the server %q without a host or with credentials", server)
		}
		if parsed.Path != "" && parsed.Path != "/" {
			t.Errorf("accepted the server %q with the path %q", server, parsed.Path)
		}
		if port := parsed.Port(); port != "" {
			if number, err := strconv.Atoi(port); err != nil || number < 1 || number > 65535 {
				t.Errorf("accepted the server %q with the port %q", server, port)
			}
		} else if strings.HasSuffix(parsed.Host, ":") {
			t.Errorf("accepted the server %q with a colon and no port", server)
		}

		if err := goRefusesRequest(server, "x"); err != nil {
			t.Errorf("accepted the server %q that Go refuses to send a request to: %v", server, err)
		}
	}

	token := file.Token
	if token == "" {
		return
	}
	if len(token) > 512 {
		t.Errorf("accepted a token of %d bytes, over the limit of 512", len(token))
	}
	for index := 0; index < len(token); index++ {
		if token[index] < '!' || token[index] > '~' {
			t.Errorf("accepted a token with the byte %#x, which is not visible ASCII", token[index])
		}
	}

	// Go refuses a header value with a control character, so a token that passes
	// here cannot add a header line to a request.
	if err := goRefusesRequest("http://example.com/", token); err != nil {
		t.Errorf("accepted a token that Go refuses as a header value: %v", err)
	}
}

// encodeUserFile writes the settings of file as the user config would hold
// them, without an empty setting, which the decoder rejects.
func encodeUserFile(t *testing.T, file UserFile) []byte {
	t.Helper()
	object := map[string]any{}
	if file.Server != "" {
		object["server"] = file.Server
	}
	if file.Token != "" {
		object["token"] = file.Token
	}
	return encodeJSON(t, object)
}

func FuzzDecodeUserFile(f *testing.F) {
	for _, seed := range userSeeds() {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, body []byte) {
		file, err := DecodeUserFile(bytes.NewReader(body))
		if err != nil {
			if file != (UserFile{}) {
				t.Fatalf("DecodeUserFile() returned %+v and the error %v", file, err)
			}
			// An error must never repeat a value that can hold a secret. The error
			// for an unknown field quotes the name of the field on purpose, so a typo
			// is easy to see, and a name is not a value. Test for the type of the
			// error, because the text of a message can come from the input.
			var unknown *strictjson.UnknownFieldError
			if errors.As(err, &unknown) {
				return
			}
			for _, secret := range []string{"hunter2", "s3cret"} {
				if bytes.Contains(body, []byte(secret)) && strings.Contains(err.Error(), secret) {
					t.Errorf("DecodeUserFile() error = %q, want it not to repeat %q", err, secret)
				}
			}
			return
		}
		if len(body) > maxUserFileBytes {
			t.Errorf("accepted a file of %d bytes, over the limit of %d", len(body), maxUserFileBytes)
		}
		checkAcceptedUser(t, file)
		checkAcceptedUserAgainstInput(t, body, file)

		encoded := encodeUserFile(t, file)
		again, err := DecodeUserFile(bytes.NewReader(encoded))
		if err != nil {
			t.Fatalf("DecodeUserFile() rejected the encoding of accepted settings %s: %v", encoded, err)
		}
		if file != again {
			t.Errorf("DecodeUserFile() of the encoding = %+v, want %+v", again, file)
		}
	})
}

func TestConfigFuzzSeedsReachBothSidesOfTheRules(t *testing.T) {
	tests := []struct {
		name         string
		seeds        []string
		decode       func(body string) error
		wantAccepted int
		wantRejected int
	}{
		{"project file", projectSeeds(), func(body string) error { _, err := DecodeProjectFile(strings.NewReader(body)); return err }, 6, 35},
		{"user config", userSeeds(), func(body string) error { _, err := DecodeUserFile(strings.NewReader(body)); return err }, 7, 32},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var accepted, rejected int
			for _, seed := range test.seeds {
				if test.decode(seed) == nil {
					accepted++
				} else {
					rejected++
				}
			}
			if accepted < test.wantAccepted || rejected < test.wantRejected {
				t.Errorf("seeds hold %d accepted and %d rejected inputs, want at least %d and %d", accepted, rejected, test.wantAccepted, test.wantRejected)
			}
		})
	}
}
