package scan

import "testing"

func TestIgnoredByDefault(t *testing.T) {
	tests := []struct {
		name  string
		entry string
		isDir bool
		want  bool
	}{
		{"git directory", ".git", true, true},
		{"git file of a worktree", ".git", false, true},
		{"node_modules directory", "node_modules", true, true},
		{"node_modules file", "node_modules", false, false},
		{"ds store file", ".DS_Store", false, true},
		{"ship8t config file", "ship8t.json", false, true},
		{"ship8t config directory", "ship8t.json", true, true},
		{"dotfile", ".env", false, true},
		{"dot directory", ".cache", true, true},
		{"well-known directory", ".well-known", true, false},
		{"well-known file", ".well-known", false, true},
		{"dotfile that starts like well-known", ".well-known.txt", false, true},
		{"regular file", "index.html", false, false},
		{"regular directory", "assets", true, false},
		{"name that contains a dot", "app.min.js", false, false},
		{"name that resembles node_modules", "node_modules_backup", true, false},
		{"name in another case", "NODE_MODULES", true, false},
		{"config name in another case", "Ship8t.json", false, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ignoredByDefault(test.entry, test.isDir); got != test.want {
				t.Errorf("ignoredByDefault(%q, isDir=%v) = %v, want %v", test.entry, test.isDir, got, test.want)
			}
		})
	}
}

func TestRemovedByDefault(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		isDir bool
		want  bool
	}{
		{"regular file", "index.html", false, false},
		{"nested regular file", "assets/css/site.css", false, false},
		{"dotfile at the root", ".env", false, true},
		{"dotfile in a directory", "docs/.hidden", false, true},
		{"file below a dot directory", ".git/config", false, true},
		{"file deep below a dot directory", ".cache/a/b/c.txt", false, true},
		{"file below node_modules", "docs/node_modules/lib/index.js", false, true},
		{"node_modules directory", "docs/node_modules", true, true},
		{"node_modules file", "docs/node_modules", false, false},
		{"file below well-known", ".well-known/security.txt", false, false},
		{"dotfile below well-known", ".well-known/.hidden", false, true},
		{"ship8t config at any depth", "docs/ship8t.json", false, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := removedByDefault(test.path, test.isDir); got != test.want {
				t.Errorf("removedByDefault(%q, isDir=%v) = %v, want %v", test.path, test.isDir, got, test.want)
			}
		})
	}
}
