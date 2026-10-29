package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestDecodeProjectFileRejectsInvalidFiles(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantInErr []string
	}{
		// The file is committed, so a token in it must never be accepted.
		{"token", `{"token":"s3cret-value"}`, []string{"token", "version control"}},
		{"token in capitals", `{"TOKEN":"s3cret-value"}`, []string{"token", "version control"}},
		{"token after valid fields", `{"project":"demo","token":"s3cret-value"}`, []string{"token"}},

		// Field names.
		{"unknown field", `{"server":"https://example.com"}`, []string{"unknown field", "server"}},
		{"field with the wrong case", `{"Project":"demo"}`, []string{"unknown field", "case sensitive"}},
		{"repeated field", `{"project":"a","project":"b"}`, []string{"appears more than once", "project"}},
		{"unknown field in serve", `{"serve":{"mode":"spa"}}`, []string{"serve", "unknown field"}},
		{"serve field with the wrong case", `{"serve":{"SPA_fallback":true}}`, []string{"serve", "unknown field"}},
		{"repeated field in serve", `{"serve":{"spa_fallback":true,"spa_fallback":false}}`, []string{"serve", "appears more than once"}},

		// Shape of the file.
		{"empty input", ``, []string{"invalid ship8t.json"}},
		{"truncated object", `{"project":"demo"`, []string{"invalid ship8t.json"}},
		{"array", `[]`, []string{"JSON object"}},
		{"null", `null`, []string{"JSON object"}},
		{"string", `"demo"`, []string{"JSON object"}},
		{"second object", `{} {}`, []string{"unexpected data"}},
		{"text after the object", `{} trailing`, []string{"unexpected data"}},

		// project.
		{"empty project", `{"project":""}`, []string{"project"}},
		{"project with capitals", `{"project":"My-Site"}`, []string{"project", "lowercase"}},
		{"project with an underscore", `{"project":"my_site"}`, []string{"project"}},
		{"project with a dot", `{"project":"my.site"}`, []string{"project"}},
		{"project with a space", `{"project":"my site"}`, []string{"project"}},
		{"project with a slash", `{"project":"a/b"}`, []string{"project"}},
		{"project with a non-ascii letter", `{"project":"café"}`, []string{"project"}},
		{"project that starts with a hyphen", `{"project":"-site"}`, []string{"project", "hyphen"}},
		{"project that ends with a hyphen", `{"project":"site-"}`, []string{"project", "hyphen"}},
		{"project of 64 characters", `{"project":"` + strings.Repeat("a", 64) + `"}`, []string{"project", "63"}},
		{"project that is a number", `{"project":1}`, []string{"project"}},
		{"project that is null", `{"project":null}`, []string{"project", "null"}},

		// directory. The file can come from a repository that the user did not
		// write, so the directory must stay inside the project directory.
		{"empty directory", `{"directory":""}`, []string{"directory", "empty"}},
		{"absolute directory", `{"directory":"/etc"}`, []string{"directory", "leading /"}},
		{"root directory", `{"directory":"/"}`, []string{"directory", "leading /"}},
		{"parent directory", `{"directory":".."}`, []string{"directory", "inside the project directory"}},
		{"parent directory below a name", `{"directory":"dist/.."}`, []string{"directory", "inside the project directory"}},
		{"parent directory in the middle", `{"directory":"a/../b"}`, []string{"directory", "inside the project directory"}},
		{"directory that climbs out", `{"directory":"../dist"}`, []string{"directory", "inside the project directory"}},
		{"directory that climbs out deeply", `{"directory":"../../../etc"}`, []string{"directory", "inside the project directory"}},
		{"directory with a current directory segment", `{"directory":"./dist"}`, []string{"directory", "clean relative path"}},
		{"directory with an empty segment", `{"directory":"a//b"}`, []string{"directory", "clean relative path"}},
		{"directory with a trailing slash", `{"directory":"dist/"}`, []string{"directory", "clean relative path"}},
		{"directory with a backslash", `{"directory":"a\\b"}`, []string{"directory", "backslash"}},
		{"directory with a windows drive and backslash", `{"directory":"C:\\site"}`, []string{"directory", "backslash"}},
		{"directory with a control character", `{"directory":"a\u0000b"}`, []string{"directory", "control"}},
		{"directory with a newline", `{"directory":"a\nb"}`, []string{"directory", "control"}},
		{"directory with the replacement character", `{"directory":"a\ufffdb"}`, []string{"directory", "UTF-8"}},
		{"directory over 1024 bytes", `{"directory":"` + strings.Repeat("a", 1025) + `"}`, []string{"directory", "1024"}},
		{"directory that is a number", `{"directory":1}`, []string{"directory"}},
		{"directory that is null", `{"directory":null}`, []string{"directory", "null"}},

		// ignore and include.
		{"ignore that is a string", `{"ignore":"*.map"}`, []string{"ignore"}},
		{"ignore that is an object", `{"ignore":{}}`, []string{"ignore"}},
		{"ignore that is null", `{"ignore":null}`, []string{"ignore", "null"}},
		{"ignore with a number", `{"ignore":["a",1]}`, []string{"ignore"}},
		{"ignore with a null entry", `{"ignore":["a",null]}`, []string{"ignore[1]", "null"}},
		{"include that is a string", `{"include":".env"}`, []string{"include"}},
		{"include with a null entry", `{"include":[null]}`, []string{"include[0]", "null"}},
		{"include that is null", `{"include":null}`, []string{"include", "null"}},

		// serve.
		{"serve that is a string", `{"serve":"spa"}`, []string{"serve"}},
		{"serve that is null", `{"serve":null}`, []string{"serve", "null"}},
		{"spa_fallback that is a string", `{"serve":{"spa_fallback":"yes"}}`, []string{"spa_fallback"}},
		{"spa_fallback that is a number", `{"serve":{"spa_fallback":1}}`, []string{"spa_fallback"}},
		{"spa_fallback that is null", `{"serve":{"spa_fallback":null}}`, []string{"spa_fallback", "null"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := DecodeProjectFile(strings.NewReader(test.body))
			if err == nil {
				t.Fatalf("DecodeProjectFile(%s) = %+v, nil error, want an error", test.body, got)
			}
			if !reflect.DeepEqual(got, ProjectFile{}) {
				t.Errorf("DecodeProjectFile(%s) = %+v with an error, want the zero value", test.body, got)
			}
			for _, want := range test.wantInErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("DecodeProjectFile(%s) error = %q, want it to contain %q", test.body, err, want)
				}
			}
			if strings.Contains(err.Error(), "s3cret") {
				t.Errorf("DecodeProjectFile(%s) error = %q, want it not to repeat the token", test.body, err)
			}
		})
	}
}

func TestDecodeProjectFileAcceptsValidFiles(t *testing.T) {
	tests := []struct {
		name string
		body string
		want ProjectFile
	}{
		{"empty object", `{}`, ProjectFile{}},
		{"surrounding whitespace", " \n\t{ }\n ", ProjectFile{}},
		{
			name: "every setting",
			body: `{"project":"my-site","directory":"dist","ignore":["**/*.map","drafts/"],"include":[".well-known/**"],"serve":{"spa_fallback":true}}`,
			want: ProjectFile{
				Project:   "my-site",
				Directory: "dist",
				Ignore:    []string{"**/*.map", "drafts/"},
				Include:   []string{".well-known/**"},
				Serve:     Serve{SPAFallback: true},
			},
		},
		{"settings in any order", `{"serve":{},"include":[],"directory":"public","project":"a"}`, ProjectFile{Project: "a", Directory: "public", Ignore: nil, Include: []string{}}},
		{"project of one character", `{"project":"a"}`, ProjectFile{Project: "a"}},
		{"project of digits", `{"project":"123"}`, ProjectFile{Project: "123"}},
		{"project with hyphens inside", `{"project":"a-b-c"}`, ProjectFile{Project: "a-b-c"}},
		{"project of 63 characters", `{"project":"` + strings.Repeat("a", 63) + `"}`, ProjectFile{Project: strings.Repeat("a", 63)}},
		{"directory that is the project directory", `{"directory":"."}`, ProjectFile{Directory: "."}},
		{"directory below the project", `{"directory":"build/public"}`, ProjectFile{Directory: "build/public"}},
		{"directory with dots in names", `{"directory":"a.b/c..d/.e"}`, ProjectFile{Directory: "a.b/c..d/.e"}},
		{"directory of 1024 bytes", `{"directory":"` + strings.Repeat("a", 1024) + `"}`, ProjectFile{Directory: strings.Repeat("a", 1024)}},
		{"empty ignore list", `{"ignore":[]}`, ProjectFile{Ignore: []string{}}},
		{"spa_fallback off", `{"serve":{"spa_fallback":false}}`, ProjectFile{}},
		{"empty serve object", `{"serve":{}}`, ProjectFile{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := DecodeProjectFile(strings.NewReader(test.body))
			if err != nil {
				t.Fatalf("DecodeProjectFile(%s) error = %v", test.body, err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("DecodeProjectFile(%s) = %+v, want %+v", test.body, got, test.want)
			}
		})
	}
}

func TestDecodeProjectFileKeepsPatternsAsWritten(t *testing.T) {
	// The scanner checks the pattern syntax, so the decoder passes every string
	// on, including patterns that the scanner will reject.
	got, err := DecodeProjectFile(strings.NewReader(`{"ignore":["!keep.html","/abs"," "],"include":[""]}`))
	if err != nil {
		t.Fatalf("DecodeProjectFile() error = %v", err)
	}
	want := ProjectFile{Ignore: []string{"!keep.html", "/abs", " "}, Include: []string{""}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DecodeProjectFile() = %+v, want %+v", got, want)
	}
}

func TestDecodeProjectFileSizeLimit(t *testing.T) {
	// padded returns a valid file with exactly size bytes.
	padded := func(size int) string {
		const frame = `{"ignore":["` + `"]}`
		return `{"ignore":["` + strings.Repeat("a", size-len(frame)) + `"]}`
	}

	if _, err := DecodeProjectFile(strings.NewReader(padded(maxProjectFileBytes))); err != nil {
		t.Errorf("DecodeProjectFile() of exactly %d bytes error = %v, want nil", maxProjectFileBytes, err)
	}

	_, err := DecodeProjectFile(strings.NewReader(padded(maxProjectFileBytes + 1)))
	if err == nil || !strings.Contains(err.Error(), "larger than 65536 bytes") {
		t.Errorf("DecodeProjectFile() of %d bytes error = %v, want it to name the limit of 65536 bytes", maxProjectFileBytes+1, err)
	}
}

// endlessReader returns the letter a for ever and counts the bytes it gave.
type endlessReader struct{ given int }

func (reader *endlessReader) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = 'a'
	}
	reader.given += len(buffer)
	return len(buffer), nil
}

func TestDecodeProjectFileStopsReadingAnEndlessInput(t *testing.T) {
	reader := &endlessReader{}

	if _, err := DecodeProjectFile(reader); err == nil {
		t.Fatal("DecodeProjectFile() of an endless input = nil error, want an error")
	}
	if reader.given > maxProjectFileBytes+1 {
		t.Errorf("DecodeProjectFile() read %d bytes, want at most %d", reader.given, maxProjectFileBytes+1)
	}
}

func TestDecodeProjectFileCapsErrorsThatQuoteInput(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"unknown field name", `{"` + strings.Repeat("x", 60000) + `":1}`},
		{"project value", `{"project":"` + strings.Repeat("A", 60000) + `"}`},
		{"directory value", `{"directory":"` + strings.Repeat("a", 60000) + `/.."}`},
		{"type mismatch", `{"project":` + strings.Repeat("9", 60000) + `}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeProjectFile(strings.NewReader(test.body))
			if err == nil {
				t.Fatal("DecodeProjectFile() = nil error, want an error")
			}
			if len(err.Error()) > 400 {
				t.Errorf("DecodeProjectFile() error is %d bytes, want it capped near 400", len(err.Error()))
			}
		})
	}
}
