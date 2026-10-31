package config

import (
	"strings"
	"testing"
)

func TestReadEnvironment(t *testing.T) {
	tests := []struct {
		name    string
		server  string
		token   string
		project string
		want    environmentSettings
	}{
		{
			name:    "all three variables set",
			server:  "https://deploy.example.com",
			token:   "s3cret-value",
			project: "my-site",
			want:    environmentSettings{Server: "https://deploy.example.com", Token: "s3cret-value", Project: "my-site"},
		},
		{name: "no variable set gives no settings"},
		{name: "server only", server: "http://127.0.0.1:8481", want: environmentSettings{Server: "http://127.0.0.1:8481"}},
		{name: "token only", token: "s3cret-value", want: environmentSettings{Token: "s3cret-value"}},
		{name: "project only", project: "my-site", want: environmentSettings{Project: "my-site"}},
		{
			name:    "an empty variable is unset and the others stay",
			server:  "",
			token:   "s3cret-value",
			project: "",
			want:    environmentSettings{Token: "s3cret-value"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := readEnvironment(test.server, test.token, test.project)
			if err != nil {
				t.Fatalf("readEnvironment(%q, %q, %q) error = %v", test.server, test.token, test.project, err)
			}
			if got != test.want {
				t.Errorf("readEnvironment(%q, %q, %q) = %+v, want %+v", test.server, test.token, test.project, got, test.want)
			}
		})
	}
}

func TestReadEnvironmentRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name         string
		server       string
		token        string
		project      string
		wantVariable string
		wantPhrase   string
	}{
		{name: "server without a scheme", server: "deploy.example.com", wantVariable: "SHIP8T_SERVER", wantPhrase: "must start with https:// or http://"},
		{name: "server with a path", server: "https://deploy.example.com/api", wantVariable: "SHIP8T_SERVER", wantPhrase: "must not have a path"},
		{name: "server with credentials", server: "https://ana:hunter2@deploy.example.com", wantVariable: "SHIP8T_SERVER", wantPhrase: "must not contain a user name or password"},
		{name: "token with a space", token: "bad token", wantVariable: "SHIP8T_TOKEN", wantPhrase: "must use only visible ASCII characters"},
		{name: "token with a newline", token: "bad\nX-Header: 1", wantVariable: "SHIP8T_TOKEN", wantPhrase: "must use only visible ASCII characters"},
		{name: "project with capitals", project: "My-Site", wantVariable: "SHIP8T_PROJECT", wantPhrase: "must use only lowercase letters, digits, and hyphens"},
		{name: "project that ends in a hyphen", project: "my-site-", wantVariable: "SHIP8T_PROJECT", wantPhrase: "must not start or end with a hyphen"},
		// Trimming would hide a stray space, so the validators see it and reject it.
		{name: "a space is not an empty value", server: " ", wantVariable: "SHIP8T_SERVER", wantPhrase: "must start with https:// or http://"},
		{name: "a space token is not an empty value", token: " ", wantVariable: "SHIP8T_TOKEN", wantPhrase: "must use only visible ASCII characters"},
		{name: "a space project is not an empty value", project: " ", wantVariable: "SHIP8T_PROJECT", wantPhrase: "must use only lowercase letters, digits, and hyphens"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := readEnvironment(test.server, test.token, test.project)
			if err == nil {
				t.Fatalf("readEnvironment(%q, %q, %q) = %+v, want an error", test.server, test.token, test.project, got)
			}
			if got != (environmentSettings{}) {
				t.Errorf("readEnvironment() = %+v with an error, want no settings", got)
			}
			if !strings.HasPrefix(err.Error(), test.wantVariable+": ") {
				t.Errorf("error %q does not start with %q", err, test.wantVariable+": ")
			}
			if !strings.Contains(err.Error(), test.wantPhrase) {
				t.Errorf("error %q does not contain %q", err, test.wantPhrase)
			}
		})
	}
}

func TestReadEnvironmentErrorsKeepSecretsOut(t *testing.T) {
	tests := []struct {
		name   string
		server string
		token  string
		secret string
	}{
		{name: "server password", server: "https://ana:hunter2@deploy.example.com", secret: "hunter2"},
		{name: "server user name", server: "https://ana:hunter2@deploy.example.com", secret: "ana"},
		{name: "token with a space", token: "s3cret bad-value", secret: "s3cret"},
		{name: "oversized token", token: strings.Repeat("s3cretvalue", 100), secret: "s3cretvalue"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := readEnvironment(test.server, test.token, "")
			if err == nil {
				t.Fatal("readEnvironment() error = nil, want an error")
			}
			if strings.Contains(err.Error(), test.secret) {
				t.Errorf("error %q repeats the secret %q", err, test.secret)
			}
		})
	}
}

func TestReadEnvironmentReportsTheFirstInvalidVariable(t *testing.T) {
	_, err := readEnvironment("deploy.example.com", "bad token", "My-Site")
	if err == nil {
		t.Fatal("readEnvironment() error = nil, want an error")
	}
	if !strings.HasPrefix(err.Error(), "SHIP8T_SERVER: ") {
		t.Errorf("error %q does not start with SHIP8T_SERVER, want the server checked first", err)
	}
}
