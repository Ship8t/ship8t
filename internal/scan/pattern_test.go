package scan

import (
	"strings"
	"testing"
	"time"
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
			parsed, err := parsePattern(test.pattern)
			if err != nil {
				t.Fatalf("parsePattern(%q) error = %v", test.pattern, err)
			}
			if got := parsed.matches(test.path, test.isDir); got != test.want {
				t.Errorf("pattern %q matches(%q, isDir=%v) = %v, want %v", test.pattern, test.path, test.isDir, got, test.want)
			}
		})
	}
}

func TestMayMatchBelow(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		dir     string
		want    bool
	}{
		{"file inside the directory", ".github/pages.html", ".github", true},
		{"file two levels down", ".github/a/pages.html", ".github", true},
		{"file two levels down, one level in", ".github/a/pages.html", ".github/a", true},
		{"other directory", ".github/pages.html", ".cache", false},
		{"directory deeper than the pattern", ".github/pages.html", ".github/pages.html", false},
		{"directory deeper than the pattern, no match", ".github/pages.html", ".github/a", false},
		{"pattern that names only the directory", ".github", ".github", false},
		{"double star in the middle", "a/**/b.txt", "a/x/y", true},
		{"double star at the start", "**/*.png", ".cache/images", true},
		{"double star at the end", ".cache/**", ".cache", true},
		{"star in a segment", ".gith*/pages.html", ".github", true},
		{"star does not cross a segment", ".gith*/pages.html", ".github/a", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := parsePattern(test.pattern)
			if err != nil {
				t.Fatalf("parsePattern(%q) error = %v", test.pattern, err)
			}
			if got := mayMatchBelowAny([]pattern{parsed}, test.dir); got != test.want {
				t.Errorf("pattern %q mayMatchBelowAny(%q) = %v, want %v", test.pattern, test.dir, got, test.want)
			}
		})
	}
}

func TestMatchesEntryOrParent(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		isDir   bool
		want    bool
	}{
		{"the entry itself", "a/b.txt", "a/b.txt", false, true},
		{"the parent directory", "a", "a/b.txt", false, true},
		{"a directory further up", "a", "a/b/c/d.txt", false, true},
		{"a parent that only a directory pattern names", "a/", "a/b.txt", false, true},
		{"a file is not a directory", "a/", "a", false, false},
		{"a sibling", "a", "b/a.txt", false, false},
		{"a pattern for a deeper path", "a/b/c.txt", "a/b", true, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := parsePattern(test.pattern)
			if err != nil {
				t.Fatalf("parsePattern(%q) error = %v", test.pattern, err)
			}
			if got := matchesEntryOrParent([]pattern{parsed}, test.path, test.isDir); got != test.want {
				t.Errorf("pattern %q matchesEntryOrParent(%q, isDir=%v) = %v, want %v", test.pattern, test.path, test.isDir, got, test.want)
			}
		})
	}
}

func TestPatternMatchesManyDoubleStarsQuickly(t *testing.T) {
	tests := []struct {
		name        string
		doubleStars int
		segments    int
	}{
		{"10 double stars and 30 segments", 10, 30},
		{"30 double stars and 500 segments", 30, 500},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// The last segment of the path never matches, so the match has to rule
			// out every way that the double stars can cover the segments.
			text := strings.Repeat("**/", test.doubleStars) + "target"
			missing := strings.Repeat("a/", test.segments-1) + "b"
			found := strings.Repeat("a/", test.segments-1) + "target"
			parsed, err := parsePattern(text)
			if err != nil {
				t.Fatalf("parsePattern(%q) error = %v", text, err)
			}

			start := time.Now()
			if parsed.matches(missing, false) {
				t.Errorf("pattern %q matches %q, want no match", text, missing)
			}
			if !parsed.matches(found, false) {
				t.Errorf("pattern %q does not match %q, want a match", text, found)
			}
			// The limit is far above the time of a correct match, so a slow
			// machine cannot fail the test, and a match that tries every split
			// cannot pass it.
			if elapsed := time.Since(start); elapsed > 2*time.Second {
				t.Errorf("matching took %v, want under 2s", elapsed)
			}
		})
	}
}
