package scan

import "strings"

// ignoredByDefault reports whether the default rules of ADR 0001 skip an entry
// with the given name. It matches the name only, at any depth.
func ignoredByDefault(name string, isDir bool) bool {
	switch name {
	case "ship8t.json":
		return true
	case "node_modules":
		// The rule has a trailing slash, so it matches directories only.
		return isDir
	case ".well-known":
		// Keep the one dot directory that sites need, and skip a file of that name.
		return !isDir
	}
	return strings.HasPrefix(name, ".")
}
