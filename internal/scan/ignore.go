package scan

import "strings"

// removedByDefault reports whether a default rule removes the entry at path,
// either by its own name or because a directory above it is removed.
func removedByDefault(path string, isDir bool) bool {
	segments := strings.Split(path, "/")
	for index, segment := range segments {
		// Every segment before the last names a directory.
		isLast := index == len(segments)-1
		if ignoredByDefault(segment, !isLast || isDir) {
			return true
		}
	}
	return false
}

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
