package scan

import (
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

// patternSeed is one starting input for the pattern fuzz targets.
type patternSeed struct {
	pattern string
	path    string
	isDir   bool
}

// patternSeeds returns patterns that match and patterns that do not, together
// with every kind of invalid pattern, so the fuzzer starts from both sides of
// each rule of ADR 0001, section 7.
func patternSeeds() []patternSeed {
	return []patternSeed{
		{"robots.txt", "robots.txt", false},
		{"robots.txt", "docs/robots.txt", false},
		{"*.map", "app.map", false},
		{"*.map", "assets/app.map", false},
		{"**/*.map", "app.map", false},
		{"**/*.map", "a/b/c.map", false},
		{"a?c", "abc", false},
		{"a?c", "a/c", false},
		{"?", "é", false},
		{"a*b*c", "aXbYc", false},
		{"a/**/b", "a/b", false},
		{"a/**/b", "a/x/y/b", false},
		{"assets/**", "assets", true},
		{"**", "a/b/c.txt", false},
		{"build/", "build", true},
		{"build/", "build", false},
		{"docs/**/", "docs/a", true},
		{"**/**/a", "a", false},
		{"a/**/**/b", "a/b", false},
		{"**/a/**/b/**", "x/a/y/b/z", false},
		{"**/**/**/a", "b/b/a/b", false},
		{"assets/**", "other/a.js", false},
		{"*", "a/b", false},
		// Invalid patterns.
		{"", "a", false},
		{"/", "a", false},
		{"//", "a", false},
		{"/abs", "abs", false},
		{"a//b", "a/b", false},
		{"!keep.html", "keep.html", false},
		{"a**", "ab", false},
		{"**a", "ab", false},
		{"a/**b/c", "a/b/c", false},
		{"***", "a", false},
	}
}

// shouldRejectPattern reports whether the rules of ADR 0001, section 7, make
// text an invalid pattern. It restates the rules apart from parsePattern.
func shouldRejectPattern(text string) bool {
	if strings.HasPrefix(text, "!") || strings.HasPrefix(text, "/") {
		return true
	}
	for _, segment := range strings.Split(strings.TrimSuffix(text, "/"), "/") {
		if segment == "" {
			return true
		}
		if segment != "**" && strings.Contains(segment, "**") {
			return true
		}
	}
	return false
}

// referenceMatch decides whether a valid pattern matches a path by turning the
// pattern into a regular expression. It shares no code with pattern.matches,
// so a mistake in one of them shows as a difference between the two. It needs
// valid UTF-8, because the regular expression library rejects anything else.
func referenceMatch(t *testing.T, text, path string, isDir bool) bool {
	t.Helper()
	body, directoriesOnly := strings.CutSuffix(text, "/")
	if directoriesOnly && !isDir {
		return false
	}

	// Match "/" plus the path, so every segment starts with a slash and ** can
	// stand for any number of whole segments.
	var expression strings.Builder
	expression.WriteString("^")
	for _, segment := range strings.Split(body, "/") {
		if segment == "**" {
			expression.WriteString("(?:/[^/]*)*")
			continue
		}
		expression.WriteString("/")
		for _, character := range segment {
			switch character {
			case '*':
				expression.WriteString("[^/]*")
			case '?':
				expression.WriteString("[^/]")
			default:
				expression.WriteString(regexp.QuoteMeta(string(character)))
			}
		}
	}
	expression.WriteString("$")

	compiled, err := regexp.Compile(expression.String())
	if err != nil {
		t.Fatalf("reference expression %q for pattern %q does not compile: %v", expression.String(), text, err)
	}
	return compiled.MatchString("/" + path)
}

func FuzzPatternMatches(f *testing.F) {
	for _, seed := range patternSeeds() {
		f.Add(seed.pattern, seed.path, seed.isDir)
	}

	f.Fuzz(func(t *testing.T, text, path string, isDir bool) {
		parsed, err := parsePattern(text)
		if rejected := shouldRejectPattern(text); (err != nil) != rejected {
			t.Fatalf("parsePattern(%q) error = %v, but the rules say rejected = %v", text, err, rejected)
		}
		if err != nil {
			return
		}

		// Never panic, and finish, whatever the path holds.
		got := parsed.matches(path, isDir)

		if !utf8.ValidString(text) || !utf8.ValidString(path) {
			return
		}
		if want := referenceMatch(t, text, path, isDir); got != want {
			t.Errorf("pattern %q matches(%q, isDir=%v) = %v, the reference says %v", text, path, isDir, got, want)
		}
	})
}

// hasEmptySegment reports whether a slash-separated path has an empty segment.
// Paths that the walk produces never do.
func hasEmptySegment(path string) bool {
	return slices.Contains(strings.Split(path, "/"), "")
}

func FuzzMayMatchBelow(f *testing.F) {
	f.Add(".github/pages.html", ".github", "pages.html")
	f.Add("a/**/b.txt", "a/x", "b.txt")
	f.Add("**/*.png", "node_modules/x", "y.png")
	f.Add(".cache", ".cache", "a.txt")
	f.Add(".cache/", ".cache", "a")
	f.Add("a*/b", "abc", "b")
	f.Add("a?/b", "ab", "b")
	f.Add("assets/**", "assets", "x/y.js")
	f.Add("a/b", "a", "b")

	f.Fuzz(func(t *testing.T, text, directory, rest string) {
		parsed, err := parsePattern(text)
		if err != nil {
			return
		}
		below := directory + "/" + rest
		if hasEmptySegment(directory) || hasEmptySegment(below) {
			return
		}

		// A pattern that matches a path below a directory must keep the directory
		// open. A wrong answer here makes include drop a file silently.
		for _, isDir := range []bool{false, true} {
			if parsed.matches(below, isDir) && !mayMatchBelowAny([]pattern{parsed}, directory) {
				t.Errorf("pattern %q matches %q (isDir=%v) but mayMatchBelowAny(%q) = false", text, below, isDir, directory)
			}
		}
	})
}

func TestPatternSeedsReachBothSidesOfThePatternRules(t *testing.T) {
	var invalid, matched, unmatched int
	for _, seed := range patternSeeds() {
		parsed, err := parsePattern(seed.pattern)
		switch {
		case err != nil:
			invalid++
		case parsed.matches(seed.path, seed.isDir):
			matched++
		default:
			unmatched++
		}
	}
	if invalid < 8 || matched < 8 || unmatched < 5 {
		t.Errorf("seeds hold %d invalid, %d matching, and %d non-matching patterns, want at least 8, 8, and 5", invalid, matched, unmatched)
	}
}
