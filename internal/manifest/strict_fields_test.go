package manifest

import (
	"strings"
	"testing"
)

func TestDecodeRejectsFieldNamesWithTheWrongCase(t *testing.T) {
	file := func(fields string) string { return filesBody(`{` + fields + `}`) }
	checksumField := `"sha256":"` + validChecksum + `"`

	tests := []struct {
		name string
		body string
	}{
		{"capitalized size", file(`"path":"a.html","Size":1,` + checksumField)},
		{"upper case path", file(`"PATH":"a.html","size":1,` + checksumField)},
		{"upper case checksum field", file(`"path":"a.html","size":1,"SHA256":"` + validChecksum + `"`)},
		{"capitalized files", `{"manifest_version":1,"Files":[` + validFile + `]}`},
		{"capitalized version", `{"Manifest_Version":1,"files":[` + validFile + `]}`},
		{"capitalized serve", `{"manifest_version":1,"files":[` + validFile + `],"Serve":{}}`},
		{"capitalized spa fallback", `{"manifest_version":1,"files":[` + validFile + `],"serve":{"Spa_Fallback":true}}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Decode(strings.NewReader(test.body), DefaultLimits())
			if err == nil {
				t.Fatalf("Decode(%s) = nil error, want an error", test.body)
			}
			if !strings.Contains(err.Error(), "unknown field") {
				t.Errorf("error %q does not say unknown field", err)
			}
		})
	}
}

func TestDecodeRejectsRepeatedFieldNames(t *testing.T) {
	checksumField := `"sha256":"` + validChecksum + `"`

	tests := []struct {
		name string
		body string
	}{
		{"repeated size", filesBody(`{"path":"a.html","size":5,"size":6,` + checksumField + `}`)},
		{"repeated size with null last", filesBody(`{"path":"a.html","size":5,"size":null,` + checksumField + `}`)},
		{"repeated size with null first", filesBody(`{"path":"a.html","size":null,"size":5,` + checksumField + `}`)},
		{"repeated files with null first", `{"manifest_version":1,"files":null,"files":[` + validFile + `]}`},
		{"repeated version with null first", `{"manifest_version":null,"manifest_version":1,"files":[` + validFile + `]}`},
		{"repeated serve with null first", `{"manifest_version":1,"files":[` + validFile + `],"serve":null,"serve":{}}`},
		{"repeated spa fallback with null first", `{"manifest_version":1,"files":[` + validFile + `],"serve":{"spa_fallback":null,"spa_fallback":true}}`},
		{"repeated path", filesBody(`{"path":"a.html","path":"b.html","size":1,` + checksumField + `}`)},
		{"repeated checksum", filesBody(`{"path":"a.html","size":1,` + checksumField + `,` + checksumField + `}`)},
		{"repeated files", `{"manifest_version":1,"files":[` + validFile + `],"files":[` + validFile + `]}`},
		{"repeated version", `{"manifest_version":1,"manifest_version":1,"files":[` + validFile + `]}`},
		{"repeated serve", `{"manifest_version":1,"files":[` + validFile + `],"serve":{},"serve":{}}`},
		{"repeated spa fallback", `{"manifest_version":1,"files":[` + validFile + `],"serve":{"spa_fallback":true,"spa_fallback":false}}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Decode(strings.NewReader(test.body), DefaultLimits())
			if err == nil {
				t.Fatalf("Decode(%s) = nil error, want an error", test.body)
			}
			if !strings.Contains(err.Error(), "more than once") {
				t.Errorf("error %q does not say the field repeats", err)
			}
		})
	}
}

func TestDecodeRejectsObjectsThatAreNotObjects(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"null serve", `{"manifest_version":1,"files":[` + validFile + `],"serve":null}`},
		{"array serve", `{"manifest_version":1,"files":[` + validFile + `],"serve":[]}`},
		{"string serve", `{"manifest_version":1,"files":[` + validFile + `],"serve":"on"}`},
		{"null file", filesBody(`null`)},
		{"number file", filesBody(`5`)},
		{"array file", filesBody(`[]`)},
		{"string spa fallback", `{"manifest_version":1,"files":[` + validFile + `],"serve":{"spa_fallback":"yes"}}`},
		{"string version", `{"manifest_version":"1","files":[` + validFile + `]}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Decode(strings.NewReader(test.body), DefaultLimits()); err == nil {
				t.Fatalf("Decode(%s) = nil error, want an error", test.body)
			}
		})
	}
}

func TestDecodeNamesTheFieldsItAccepts(t *testing.T) {
	body := filesBody(`{"path":"a.html","Size":1,"sha256":"` + validChecksum + `"}`)

	_, err := Decode(strings.NewReader(body), DefaultLimits())
	if err == nil {
		t.Fatal("Decode returned nil, want an error")
	}
	for _, want := range []string{"files[0]", `"Size"`, "case sensitive", "path, size, sha256"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestDecodeDoesNotEchoAnOversizedFieldName(t *testing.T) {
	body := `{"manifest_version":1,"` + strings.Repeat("k", 1<<20) + `":1,"files":[` + validFile + `]}`

	_, err := Decode(strings.NewReader(body), DefaultLimits())
	if err == nil {
		t.Fatal("Decode returned nil, want an error")
	}
	if len(err.Error()) > 300 {
		t.Fatalf("error is %d bytes long, want it to stay short", len(err.Error()))
	}
}

func TestDecodeAcceptsFieldsInAnyOrder(t *testing.T) {
	body := `{"serve":{"spa_fallback":true},"files":[{"sha256":"` + validChecksum + `","size":3,"path":"a.html"}],"manifest_version":1}`

	got, err := Decode(strings.NewReader(body), DefaultLimits())
	if err != nil {
		t.Fatalf("Decode returned %v, want nil", err)
	}
	if want := (File{Path: "a.html", Size: 3, SHA256: validChecksum}); got.Files[0] != want {
		t.Errorf("Files[0] = %+v, want %+v", got.Files[0], want)
	}
	if !got.Serve.SPAFallback {
		t.Error("Serve.SPAFallback = false, want true")
	}
}

func TestDecodeAcceptsAnEmptyServeObject(t *testing.T) {
	body := `{"manifest_version":1,"files":[` + validFile + `],"serve":{}}`

	got, err := Decode(strings.NewReader(body), DefaultLimits())
	if err != nil {
		t.Fatalf("Decode returned %v, want nil", err)
	}
	if got.Serve.SPAFallback {
		t.Error("Serve.SPAFallback = true, want false")
	}
}

func TestDecodeStrictObjectRejectsMalformedJSON(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"empty input", ``},
		{"open brace only", `{`},
		{"key without a value", `{"path"`},
		{"key with a colon only", `{"path":`},
		{"value without a closing brace", `{"path":"a"`},
		{"array", `[]`},
		{"null", `null`},
		{"number", `5`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := decodeStrictObject([]byte(test.data), "path"); err == nil {
				t.Fatalf("decodeStrictObject(%q) = nil error, want an error", test.data)
			}
		})
	}
}

func TestFileListRejectsMalformedJSON(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"empty input", ``},
		{"unclosed array", `[`},
		{"unclosed element", `[{"size":0`},
		{"missing closing bracket", `[{"size":0}`},
		{"not an array", `{}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			list := fileList{maxFiles: 10}
			if err := list.UnmarshalJSON([]byte(test.data)); err == nil {
				t.Fatalf("UnmarshalJSON(%q) = nil error, want an error", test.data)
			}
		})
	}
}

func TestDecodeReportsTheFirstFieldWithAWrongType(t *testing.T) {
	// Fields are read in a fixed order, so the same body always gives the same error.
	body := filesBody(`{"path":5,"size":"x","sha256":7}`)

	for attempt := 0; attempt < 20; attempt++ {
		_, err := Decode(strings.NewReader(body), DefaultLimits())
		if err == nil || !strings.Contains(err.Error(), "path:") {
			t.Fatalf("Decode error = %v, want an error about path", err)
		}
	}
}

func TestDecodeComparesFieldNamesAfterUnescaping(t *testing.T) {
	// \u0073 spells s and \u0053 spells S, so these bodies use real JSON escapes.
	checksumField := `"sha256":"` + validChecksum + `"`

	tests := []struct {
		name    string
		file    string
		wantErr string
	}{
		{"escaped spelling of size", `{"path":"a.html","\u0073ize":1,` + checksumField + `}`, ""},
		{"escape in the middle of a name", `{"path":"a.html","si\u007ae":1,` + checksumField + `}`, ""},
		{"escaped spelling repeats a plain size", `{"path":"a.html","size":1,"\u0073ize":2,` + checksumField + `}`, "more than once"},
		{"plain spelling repeats an escaped size", `{"path":"a.html","\u0073ize":1,"size":2,` + checksumField + `}`, "more than once"},
		{"escaped capital letter", `{"path":"a.html","\u0053ize":1,` + checksumField + `}`, `unknown field "Size"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Decode(strings.NewReader(filesBody(test.file)), DefaultLimits())
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("Decode returned %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Decode error = %v, want it to contain %q", err, test.wantErr)
			}
		})
	}
}

func TestDecodeDoesNotEchoAnOversizedNumber(t *testing.T) {
	// The text of a JSON type error can include the whole number, which is
	// untrusted and as long as the body cap allows.
	digits := strings.Repeat("9", 1<<20)

	tests := []struct {
		name string
		body string
	}{
		{"manifest version", `{"manifest_version":` + digits + `,"files":[` + validFile + `]}`},
		{"file size out of range", filesBody(`{"path":"a.html","size":` + digits + `,"sha256":"` + validChecksum + `"}`)},
		{"exponent number as a size", filesBody(`{"path":"a.html","size":1e` + digits + `,"sha256":"` + validChecksum + `"}`)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Decode(strings.NewReader(test.body), DefaultLimits())
			if err == nil {
				t.Fatal("Decode returned nil, want an error")
			}
			// The capped errors are about 135 bytes, so this limit leaves little room above them.
			if len(err.Error()) > 160 {
				t.Fatalf("error is %d bytes long, want it to stay short", len(err.Error()))
			}
		})
	}
}

func TestDecodeKeepsOrdinaryTypeErrorsReadable(t *testing.T) {
	body := filesBody(`{"path":"a.html","size":"x","sha256":"` + validChecksum + `"}`)

	_, err := Decode(strings.NewReader(body), DefaultLimits())
	if err == nil {
		t.Fatal("Decode returned nil, want an error")
	}
	// The tail of the message proves the cap does not cut ordinary errors.
	for _, want := range []string{"files[0]", "size", "cannot unmarshal string", "into Go value of type int64"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}
