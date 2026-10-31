package config

import "fmt"

// environmentSettings holds the settings that the environment variables give.
// An empty field means that the variable is unset.
type environmentSettings struct {
	Server  string
	Token   string
	Project string
}

// readEnvironment validates the values of SHIP8T_SERVER, SHIP8T_TOKEN, and
// SHIP8T_PROJECT with the same rules as the config files. The caller passes the
// values from os.Getenv. It checks the server first, then the token, then the
// project, and it reports only the first invalid variable.
func readEnvironment(server, token, project string) (environmentSettings, error) {
	if err := validateVariable("SHIP8T_SERVER", server, validateServer); err != nil {
		return environmentSettings{}, err
	}
	if err := validateVariable("SHIP8T_TOKEN", token, validateToken); err != nil {
		return environmentSettings{}, err
	}
	if err := validateVariable("SHIP8T_PROJECT", project, validateProject); err != nil {
		return environmentSettings{}, err
	}
	return environmentSettings{Server: server, Token: token, Project: project}, nil
}

// validateVariable reports why the value of the variable called name is not
// valid, and names the variable in the error so the user knows where it came
// from. It treats an empty value as unset and does not trim, so a stray space
// reaches the validator and is rejected.
func validateVariable(name, value string, validate func(string) error) error {
	if value == "" {
		return nil
	}
	if err := validate(value); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}
