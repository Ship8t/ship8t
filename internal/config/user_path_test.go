package config

import (
	"strings"
	"testing"
)

func TestUserConfigDirectory(t *testing.T) {
	tests := []struct {
		name          string
		xdgConfigHome string
		home          string
		want          string
		wantError     bool
	}{
		{name: "absolute xdg wins over home", xdgConfigHome: "/xdg/config", home: "/home/ana", want: "/xdg/config/ship8t"},
		{name: "empty xdg falls back to the home config directory", xdgConfigHome: "", home: "/home/ana", want: "/home/ana/.config/ship8t"},
		{name: "relative xdg is ignored", xdgConfigHome: "config", home: "/home/ana", want: "/home/ana/.config/ship8t"},
		{name: "dot relative xdg is ignored", xdgConfigHome: "./config", home: "/home/ana", want: "/home/ana/.config/ship8t"},
		{name: "absolute xdg needs no home", xdgConfigHome: "/xdg/config", home: "", want: "/xdg/config/ship8t"},
		{name: "relative xdg with empty home is an error", xdgConfigHome: "config", home: "", wantError: true},
		{name: "empty xdg with empty home is an error", xdgConfigHome: "", home: "", wantError: true},
		{name: "empty xdg with relative home is an error", xdgConfigHome: "", home: "home/ana", wantError: true},
		{name: "relative xdg with relative home is an error", xdgConfigHome: "config", home: "./home", wantError: true},
		{name: "trailing slash in xdg is not cleaned", xdgConfigHome: "/xdg/config/", home: "/home/ana", want: "/xdg/config//ship8t"},
		{name: "dot dot in xdg is not cleaned", xdgConfigHome: "/xdg/link/..", home: "/home/ana", want: "/xdg/link/../ship8t"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := userConfigDirectory(test.xdgConfigHome, test.home)
			if test.wantError {
				if err == nil {
					t.Fatalf("userConfigDirectory(%q, %q) = %q, want an error", test.xdgConfigHome, test.home, got)
				}
				if got != "" {
					t.Errorf("userConfigDirectory(%q, %q) = %q with an error, want no directory", test.xdgConfigHome, test.home, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("userConfigDirectory(%q, %q) error = %v", test.xdgConfigHome, test.home, err)
			}
			if got != test.want {
				t.Errorf("userConfigDirectory(%q, %q) = %q, want %q", test.xdgConfigHome, test.home, got, test.want)
			}
		})
	}
}

func TestUserConfigDirectoryErrorNamesBothVariables(t *testing.T) {
	_, err := userConfigDirectory("", "")
	if err == nil {
		t.Fatal("userConfigDirectory(\"\", \"\") error = nil, want an error")
	}
	for _, variable := range []string{"XDG_CONFIG_HOME", "HOME"} {
		if !strings.Contains(err.Error(), variable) {
			t.Errorf("error %q does not name %s", err, variable)
		}
	}
}
