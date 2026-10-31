package config

// settingSource names the kind of source that gave a setting. The zero value
// means that no source gave it.
type settingSource string

const (
	sourceFlag        settingSource = "flag"
	sourceEnvironment settingSource = "environment"
	sourceProjectFile settingSource = "ship8t.json"
	sourceUserFile    settingSource = "user config"
)

// resolvedSetting holds the value of one setting and the source that gave it.
// Both are empty when no source gave the setting.
type resolvedSetting struct {
	Value  string
	Source settingSource
}

// resolvedSettings holds the server, token, and project after precedence. The
// token is a secret, so a caller must not print this struct as it is.
type resolvedSettings struct {
	Server  resolvedSetting
	Token   resolvedSetting
	Project resolvedSetting
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
		Server: firstSetting(
			resolvedSetting{flagServer, sourceFlag},
			resolvedSetting{environment.Server, sourceEnvironment},
			resolvedSetting{user.Server, sourceUserFile},
		),
		Token: firstSetting(
			resolvedSetting{environment.Token, sourceEnvironment},
			resolvedSetting{user.Token, sourceUserFile},
		),
		Project: firstSetting(
			resolvedSetting{flagProject, sourceFlag},
			resolvedSetting{environment.Project, sourceEnvironment},
			resolvedSetting{project.Project, sourceProjectFile},
		),
	}, nil
}

// firstSetting returns the first candidate that has a value, or the empty
// setting when none has one.
func firstSetting(candidates ...resolvedSetting) resolvedSetting {
	for _, candidate := range candidates {
		if candidate.Value != "" {
			return candidate
		}
	}
	return resolvedSetting{}
}
