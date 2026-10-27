package manifest

import (
	"strings"
	"testing"
)

func TestValidatePathRejectsUnsafePaths(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"empty path", ""},
		{"leading slash", "/index.html"},
		{"empty segment", "assets//app.js"},
		{"trailing slash", "assets/"},
		{"current directory only", "."},
		{"parent directory only", ".."},
		{"current directory segment", "./index.html"},
		{"parent directory segment", "../secret.txt"},
		{"parent directory segment in the middle", "assets/../../secret.txt"},
		{"backslash separator", `assets\app.js`},
		{"windows drive path", `C:\windows\system.ini`},
		{"nul character", "index\x00.html"},
		{"newline character", "index\n.html"},
		{"tab character", "in\tdex.html"},
		{"delete character", "index\x7f.html"},
		{"c1 control character", "index\u0085.html"},
		{"invalid utf-8", "index\xff.html"},
		{"decomposed accent", "cafe\u0301.html"},
		{"decomposed umlaut", "a\u0308.html"},
		{"decomposed name in a directory", "cafe\u0301/index.html"},
		{"angstrom sign", "\u212b.html"},
		{"hangul jamo sequence", "\u1100\u1161.txt"},
		{"decomposed name after an NFC segment", "dir/cafe\u0301.html"},
		{"angstrom sign in a deep segment", "a/b/\u212b.txt"},
		{"reserved name", "CON"},
		{"reserved name in lowercase", "aux"},
		{"reserved name with extension", "nul.txt"},
		{"reserved name in a directory", "assets/prn/app.js"},
		{"reserved name with a number", "com1"},
		{"reserved name with the highest number", "LPT9.log"},
		{"trailing dot", "assets/app."},
		{"trailing space", "assets/app.js "},
		{"directory with trailing dot", "assets./app.js"},
		{"path over 1024 bytes", strings.Repeat("a/", 512) + "a"},
		{"path over 1024 bytes by multibyte characters", strings.Repeat("é/", 341) + "é"},
		{"segment over 255 bytes", strings.Repeat("a", 256)},
		{"segment over 255 bytes by multibyte characters", strings.Repeat("é", 128)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidatePath(test.path); err == nil {
				t.Fatalf("ValidatePath(%q) = nil, want an error", test.path)
			}
		})
	}
}

func TestValidatePathAcceptsSafePaths(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"root file", "index.html"},
		{"nested file", "assets/css/site.css"},
		{"hidden file", ".well-known/security.txt"},
		{"dots inside a name", "app.min.js"},
		{"spaces inside a name", "my page.html"},
		{"non-ascii name", "café/menü.html"},
		{"precomposed accent", "caf\u00e9.html"},
		{"precomposed hangul syllable", "\uac00.txt"},
		{"combining mark without a precomposed form", "x\u0301.html"},
		// These are NFC but not NFKC, so they prove the rule is NFC and not NFKC.
		{"fi ligature", "\ufb01le.html"},
		{"circled digit", "\u2460.txt"},
		{"fullwidth letter", "\uff21.html"},
		{"name that only starts with a reserved name", "console.log"},
		{"reserved name with a letter suffix", "com10"},
		{"device name with the number zero", "com0"},
		{"printer name with the number zero", "lpt0.txt"},
		{"path of exactly 1024 bytes", strings.Repeat("a/", 511) + "aa"},
		{"segment of exactly 255 bytes", strings.Repeat("a", 255)},
		{"segment of exactly 255 bytes by multibyte characters", strings.Repeat("é", 127) + "a"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidatePath(test.path); err != nil {
				t.Fatalf("ValidatePath(%q) = %v, want nil", test.path, err)
			}
		})
	}
}

func TestValidatePathQuotesTheOffendingPath(t *testing.T) {
	err := ValidatePath("evil\nlog line.html")
	if err == nil {
		t.Fatal("ValidatePath returned nil, want an error")
	}
	if strings.Contains(err.Error(), "\n") {
		t.Fatalf("error %q contains a raw newline", err)
	}
}

func TestValidatePathDoesNotEchoAnOversizedPath(t *testing.T) {
	err := ValidatePath(strings.Repeat("a", 1<<20))
	if err == nil {
		t.Fatal("ValidatePath returned nil, want an error")
	}
	if len(err.Error()) > 200 {
		t.Fatalf("error is %d bytes long, want it to stay short", len(err.Error()))
	}
}

func TestValidatePathNamesTheNormalizationForm(t *testing.T) {
	err := ValidatePath("cafe\u0301.html")
	if err == nil {
		t.Fatal("ValidatePath returned nil, want an error")
	}
	if !strings.Contains(err.Error(), "NFC") {
		t.Fatalf("error %q does not name the NFC form", err)
	}
}

func TestValidatePathDoesNotEchoAnOversizedNonNFCPath(t *testing.T) {
	// The path fails the NFC check too, so only the check order keeps the error short.
	err := ValidatePath("e\u0301" + strings.Repeat("a", 1<<20))
	if err == nil {
		t.Fatal("ValidatePath returned nil, want an error")
	}
	if len(err.Error()) > 200 {
		t.Fatalf("error is %d bytes long, want it to stay short", len(err.Error()))
	}
}
