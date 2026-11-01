package storage

import (
	"strings"
	"unicode"
)

// chmodCommand returns the command that limits path to its owner. It reports
// false when the path holds a character that is not safe to print as it is,
// such as a newline, an escape, or a direction change, because the caller would
// print a command that forges a line or hides its meaning.
func chmodCommand(path string) (string, bool) {
	for _, character := range path {
		if !unicode.IsPrint(character) || character == unicode.ReplacementChar {
			return "", false
		}
	}
	// Put -- before the mode, so a relative path that starts with a dash is not
	// read as an option. BSD chmod stops reading options at the mode.
	return "chmod -- 700 " + shellQuote(path), true
}

// shellQuote wraps text in single quotes for a POSIX shell. Inside single quotes
// every character is literal except the quote, which it closes, escapes, and
// opens again.
func shellQuote(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}
