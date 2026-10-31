package config

import (
	"strings"
	"testing"
)

func TestResolveSettings(t *testing.T) {
	tests := []struct {
		name        string
		flagServer  string
		flagProject string
		environment environmentSettings
		project     ProjectFile
		user        UserFile
		want        resolvedSettings
	}{
		{name: "no source gives empty settings"},
		{
			name:        "every source set, each setting takes the highest source that can give it",
			flagServer:  "https://flag.example.com",
			flagProject: "flag-project",
			environment: environmentSettings{Server: "https://env.example.com", Token: "env-token", Project: "env-project"},
			project:     ProjectFile{Project: "file-project"},
			user:        UserFile{Server: "https://user.example.com", Token: "user-token"},
			want:        resolvedSettings{Server: "https://flag.example.com", Token: "env-token", Project: "flag-project"},
		},
		{
			name:        "server from the environment beats the user config",
			environment: environmentSettings{Server: "https://env.example.com"},
			user:        UserFile{Server: "https://user.example.com"},
			want:        resolvedSettings{Server: "https://env.example.com"},
		},
		{
			name:        "server from the flag beats the environment",
			flagServer:  "https://flag.example.com",
			environment: environmentSettings{Server: "https://env.example.com"},
			want:        resolvedSettings{Server: "https://flag.example.com"},
		},
		{
			name: "server from the user config alone",
			user: UserFile{Server: "https://user.example.com"},
			want: resolvedSettings{Server: "https://user.example.com"},
		},
		{
			name:        "token from the environment beats the user config",
			environment: environmentSettings{Token: "env-token"},
			user:        UserFile{Token: "user-token"},
			want:        resolvedSettings{Token: "env-token"},
		},
		{
			name: "token from the user config alone",
			user: UserFile{Token: "user-token"},
			want: resolvedSettings{Token: "user-token"},
		},
		{
			name:        "project from the flag beats the environment and the file",
			flagProject: "flag-project",
			environment: environmentSettings{Project: "env-project"},
			project:     ProjectFile{Project: "file-project"},
			want:        resolvedSettings{Project: "flag-project"},
		},
		{
			name:        "project from the environment beats the file",
			environment: environmentSettings{Project: "env-project"},
			project:     ProjectFile{Project: "file-project"},
			want:        resolvedSettings{Project: "env-project"},
		},
		{
			name:    "project from the file alone",
			project: ProjectFile{Project: "file-project"},
			want:    resolvedSettings{Project: "file-project"},
		},
		{
			name:        "an empty flag and an empty variable fall through",
			flagServer:  "",
			flagProject: "",
			environment: environmentSettings{},
			project:     ProjectFile{Project: "file-project"},
			user:        UserFile{Server: "https://user.example.com", Token: "user-token"},
			want:        resolvedSettings{Server: "https://user.example.com", Token: "user-token", Project: "file-project"},
		},
		{
			name:    "other project file settings do not matter",
			project: ProjectFile{Directory: "public", Ignore: []string{"*.map"}, Include: []string{".well-known/**"}},
			want:    resolvedSettings{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveSettings(test.flagServer, test.flagProject, test.environment, test.project, test.user)
			if err != nil {
				t.Fatalf("resolveSettings() error = %v", err)
			}
			if got != test.want {
				t.Errorf("resolveSettings() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestResolveSettingsRejectsInvalidFlags(t *testing.T) {
	tests := []struct {
		name         string
		flagServer   string
		flagProject  string
		wantFlag     string
		wantPhrase   string
		wantNoSecret string
	}{
		{name: "server without a scheme", flagServer: "deploy.example.com", wantFlag: "--server", wantPhrase: "must start with https:// or http://"},
		{name: "server with a path", flagServer: "https://deploy.example.com/api", wantFlag: "--server", wantPhrase: "must not have a path"},
		{name: "server with credentials", flagServer: "https://ana:hunter2@deploy.example.com", wantFlag: "--server", wantPhrase: "must not contain a user name or password", wantNoSecret: "hunter2"},
		{name: "project with capitals", flagProject: "My-Site", wantFlag: "--project", wantPhrase: "must use only lowercase letters, digits, and hyphens"},
		{name: "project that starts with a hyphen", flagProject: "-site", wantFlag: "--project", wantPhrase: "must not start or end with a hyphen"},
		// A valid flag beside the bad one proves that no partial result escapes.
		{name: "bad server flag beside a valid project flag", flagServer: "deploy.example.com", flagProject: "my-site", wantFlag: "--server", wantPhrase: "must start with https:// or http://"},
		{name: "bad project flag beside a valid server flag", flagServer: "https://deploy.example.com", flagProject: "My-Site", wantFlag: "--project", wantPhrase: "must use only lowercase letters, digits, and hyphens"},
		// Trimming would hide a stray space, so the validators see it and reject it.
		{name: "a space server flag is not an empty flag", flagServer: " ", wantFlag: "--server", wantPhrase: "must start with https:// or http://"},
		{name: "a space project flag is not an empty flag", flagProject: " ", wantFlag: "--project", wantPhrase: "must use only lowercase letters, digits, and hyphens"},
	}
	// Valid lower sources prove that the flag is checked and not skipped.
	environment := environmentSettings{Server: "https://env.example.com", Project: "env-project"}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveSettings(test.flagServer, test.flagProject, environment, ProjectFile{}, UserFile{})
			if err == nil {
				t.Fatalf("resolveSettings() = %+v, want an error", got)
			}
			if got != (resolvedSettings{}) {
				t.Errorf("resolveSettings() = %+v with an error, want no settings", got)
			}
			if !strings.HasPrefix(err.Error(), test.wantFlag+": ") {
				t.Errorf("error %q does not start with %q", err, test.wantFlag+": ")
			}
			if !strings.Contains(err.Error(), test.wantPhrase) {
				t.Errorf("error %q does not contain %q", err, test.wantPhrase)
			}
			if test.wantNoSecret != "" && strings.Contains(err.Error(), test.wantNoSecret) {
				t.Errorf("error %q repeats the secret %q", err, test.wantNoSecret)
			}
		})
	}
}

func TestResolveSettingsReportsTheServerFlagFirst(t *testing.T) {
	_, err := resolveSettings("deploy.example.com", "My-Site", environmentSettings{}, ProjectFile{}, UserFile{})
	if err == nil {
		t.Fatal("resolveSettings() error = nil, want an error")
	}
	if !strings.HasPrefix(err.Error(), "--server: ") {
		t.Errorf("error %q does not start with --server, want the server flag checked first", err)
	}
}
