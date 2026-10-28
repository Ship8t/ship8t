package scan

import (
	"fmt"
	"strings"
)

// pattern is a parsed ignore pattern from ADR 0001 section 7.
type pattern struct {
	segments        []string
	directoriesOnly bool
}

// parsePattern reads an ignore pattern. It reports an error for a pattern that
// cannot match a path inside the deploy directory or that uses syntax v1 does
// not support, so a mistake never silently ignores nothing.
func parsePattern(text string) (pattern, error) {
	if strings.HasPrefix(text, "!") {
		return pattern{}, fmt.Errorf("invalid pattern %q: negation with ! is not supported, use include to re-add paths", text)
	}
	if strings.HasPrefix(text, "/") {
		return pattern{}, fmt.Errorf("invalid pattern %q: write the pattern relative to the deploy directory, without a leading /", text)
	}
	// A trailing slash limits the pattern to directories.
	body, directoriesOnly := strings.CutSuffix(text, "/")
	segments := strings.Split(body, "/")
	for _, segment := range segments {
		if segment == "" {
			return pattern{}, fmt.Errorf("invalid pattern %q: the pattern has an empty path segment", text)
		}
		if segment != "**" && strings.Contains(segment, "**") {
			return pattern{}, fmt.Errorf("invalid pattern %q: ** must be a whole path segment, for example a/**/b", text)
		}
	}
	return pattern{segments: segments, directoriesOnly: directoriesOnly}, nil
}

// parsePatterns reads every pattern in texts and stops at the first invalid one.
func parsePatterns(texts []string) ([]pattern, error) {
	patterns := make([]pattern, 0, len(texts))
	for _, text := range texts {
		parsed, err := parsePattern(text)
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, parsed)
	}
	return patterns, nil
}

// matchesAny reports whether any of the patterns matches the entry.
func matchesAny(patterns []pattern, path string, isDir bool) bool {
	for _, candidate := range patterns {
		if candidate.matches(path, isDir) {
			return true
		}
	}
	return false
}

// matches reports whether the pattern matches the whole slash-separated path
// of an entry under the deploy directory.
func (p pattern) matches(path string, isDir bool) bool {
	if p.directoriesOnly && !isDir {
		return false
	}
	return matchSegments(p.segments, strings.Split(path, "/"))
}

// matchSegments matches pattern segments against path segments. A segment of
// ** matches zero or more path segments.
func matchSegments(patternSegments, pathSegments []string) bool {
	for len(patternSegments) > 0 {
		if patternSegments[0] == "**" {
			for skipped := 0; skipped <= len(pathSegments); skipped++ {
				if matchSegments(patternSegments[1:], pathSegments[skipped:]) {
					return true
				}
			}
			return false
		}
		if len(pathSegments) == 0 || !matchSegment(patternSegments[0], pathSegments[0]) {
			return false
		}
		patternSegments, pathSegments = patternSegments[1:], pathSegments[1:]
	}
	return len(pathSegments) == 0
}

// matchSegment matches one path segment against a pattern segment, where *
// matches any run of characters and ? matches one character.
func matchSegment(glob, name string) bool {
	globRunes, nameRunes := []rune(glob), []rune(name)
	globIndex, nameIndex := 0, 0
	// Remember the last star, so a failed match can retry with a longer run.
	starIndex, starNameIndex := -1, 0
	for nameIndex < len(nameRunes) {
		switch {
		case globIndex < len(globRunes) && globRunes[globIndex] == '*':
			starIndex, starNameIndex = globIndex, nameIndex
			globIndex++
		case globIndex < len(globRunes) && (globRunes[globIndex] == '?' || globRunes[globIndex] == nameRunes[nameIndex]):
			globIndex++
			nameIndex++
		case starIndex >= 0:
			starNameIndex++
			nameIndex = starNameIndex
			globIndex = starIndex + 1
		default:
			return false
		}
	}
	for globIndex < len(globRunes) && globRunes[globIndex] == '*' {
		globIndex++
	}
	return globIndex == len(globRunes)
}
