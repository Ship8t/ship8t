package config

// resolvedSettings holds the server, token, and project that the sources give
// after precedence. An empty field means that no source gave a value.
type resolvedSettings struct {
	Server  string
	Token   string
	Project string
}

// resolveSettings applies the precedence of ADR 0001, section 6 to the server,
// the token, and the project. An empty flag counts as not given. It validates
// the two flags, because the flag wins and reaches the dialer, and it trusts the
// other sources, which were validated when they were read. It gives an empty
// setting when no source can supply one, because each command reports the
// settings it needs.
func resolveSettings(flagServer, flagProject string, environment environmentSettings, project ProjectFile, user UserFile) (resolvedSettings, error) {
	if err := validateVariable("--server", flagServer, validateServer); err != nil {
		return resolvedSettings{}, err
	}
	if err := validateVariable("--project", flagProject, validateProject); err != nil {
		return resolvedSettings{}, err
	}

	// Skip the sources that cannot hold a setting. The ship8t.json file has no
	// server and no token, there is no token flag, and the user config has no
	// project.
	return resolvedSettings{
		Server:  firstNonEmpty(flagServer, environment.Server, user.Server),
		Token:   firstNonEmpty(environment.Token, user.Token),
		Project: firstNonEmpty(flagProject, environment.Project, project.Project),
	}, nil
}

// firstNonEmpty returns the first value that is not empty, or the empty string.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
