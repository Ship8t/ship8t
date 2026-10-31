package config

import (
	"errors"
	"os"
	"path/filepath"
)

// userConfigDirectory returns the ship8t directory of the user. It uses
// xdgConfigHome when that is an absolute path and the .config directory in
// homeDirectory otherwise, and it fails when neither gives an absolute base.
func userConfigDirectory(xdgConfigHome, homeDirectory string) (string, error) {
	// Ignore a relative value, because the XDG specification calls it invalid.
	base := xdgConfigHome
	if !filepath.IsAbs(base) {
		// Refuse to guess a directory, because a token file must not be read from a
		// place that the user did not choose.
		if !filepath.IsAbs(homeDirectory) {
			return "", errors.New("cannot find the user config directory, set XDG_CONFIG_HOME or HOME to an absolute path")
		}
		base = homeDirectory + string(os.PathSeparator) + ".config"
	}

	// Join as text and do not clean, because a ".." after a symlink leads
	// somewhere else than the cleaned path does.
	return base + string(os.PathSeparator) + "ship8t", nil
}
