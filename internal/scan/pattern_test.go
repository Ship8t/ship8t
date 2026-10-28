package scan

import (
	"strings"
	"testing"
)

func TestParsePatternRejectsInvalidPatterns(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		wantInErr string
	}{
		{"empty pattern", "", "empty"},
		{"only a slash", "/", "relative"},
		{"two slashes", "//", "relative"},
		{"leading slash", "/assets", "relative"},
		{"empty segment in the middle", "a//b", "empty"},
		{"negation", "!keep.html", "negation"},
		{"double star after a letter", "a**", "whole"},
		{"double star before a letter", "**a", "whole"},
		{"double star between letters", "a**b", "whole"},
		{"double star inside a later segment", "a/**b/c", "whole"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parsePattern(test.pattern)
			if err == nil {
				t.Fatalf("parsePattern(%q) = nil error, want an error", test.pattern)
			}
			if !strings.Contains(err.Error(), test.wantInErr) {
				t.Errorf("parsePattern(%q) error = %q, want it to contain %q", test.pattern, err, test.wantInErr)
			}
		})
	}
}

func TestPatternMatches(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		isDir   bool
		want    bool
	}{
		{"literal name", "robots.txt", "robots.txt", false, true},
		{"literal dot is not a wildcard", "robots.txt", "robotsxtxt", false, false},
		{"literal matches a directory too", "assets", "assets", true, true},
		{"literal is anchored to the root", "robots.txt", "docs/robots.txt", false, false},
		{"literal is case sensitive", "*.JPG", "photo.jpg", false, false},

		{"star matches a suffix", "*.map", "app.map", false, true},
		{"star matches nothing", "app*", "app", false, true},
		{"star does not cross a slash", "a*", "a/b", false, false},
		{"star is anchored to the root", "*.map", "assets/app.map", false, false},
		{"star inside a segment", "a*c", "abbbc", false, true},
		{"star in the middle needs the last letter", "a*c", "abbb", false, false},
		{"two stars in one segment", "a*b*c", "aXbYc", false, true},
		{"two stars in one segment miss", "a*b*c", "ac", false, false},
		{"star backtracks", "*ab", "aab", false, true},
		{"star at the start", "*a", "ba", false, true},
		{"star at the start misses", "*a", "ab", false, false},
		{"lone star matches one segment", "*", "a", false, true},
		{"lone star does not match two segments", "*", "a/b", false, false},

		{"question mark matches one letter", "a?c", "abc", false, true},
		{"question mark does not match nothing", "a?c", "ac", false, false},
		{"question mark does not match two letters", "a?c", "abbc", false, false},
		{"question mark matches one multibyte character", "?", "é", false, true},
		{"question mark does not match a slash", "a?b", "a/b", false, false},

		{"double star matches a file at the root", "**/*.map", "app.map", false, true},
		{"double star matches one directory", "**/*.map", "assets/app.map", false, true},
		{"double star matches many directories", "**/*.map", "a/b/c.map", false, true},
		{"double star keeps the suffix rule", "**/*.map", "app.js", false, false},
		{"trailing double star matches a nested file", "assets/**", "assets/x/y.js", false, true},
		{"trailing double star matches a direct child", "assets/**", "assets/a.js", false, true},
		{"trailing double star matches the directory itself", "assets/**", "assets", true, true},
		{"trailing double star misses another directory", "assets/**", "other/a.js", false, false},
		{"middle double star matches no directory", "a/**/b", "a/b", false, true},
		{"middle double star matches one directory", "a/**/b", "a/x/b", false, true},
		{"middle double star matches two directories", "a/**/b", "a/x/y/b", false, true},
		{"middle double star needs the last segment", "a/**/b", "a/c", false, false},
		{"middle double star needs whole segments", "a/**/b", "ab", false, false},
		{"lone double star matches everything", "**", "a/b/c.txt", false, true},

		{"trailing slash matches a directory", "build/", "build", true, true},
		{"trailing slash does not match a file", "build/", "build", false, false},
		{"trailing slash is anchored to the root", "build/", "src/build", true, false},
		{"trailing slash with double star matches a directory", "docs/**/", "docs/a", true, true},
		{"trailing slash with double star skips a file", "docs/**/", "docs/a", false, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pattern, err := parsePattern(test.pattern)
			if err != nil {
				t.Fatalf("parsePattern(%q) error = %v", test.pattern, err)
			}
			if got := pattern.matches(test.path, test.isDir); got != test.want {
				t.Errorf("pattern %q matches(%q, isDir=%v) = %v, want %v", test.pattern, test.path, test.isDir, got, test.want)
			}
		})
	}
}
