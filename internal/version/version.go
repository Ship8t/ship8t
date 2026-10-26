// Package version reports the build version of the Ship8t binaries.
package version

import "runtime/debug"

// stampedVersion is set at build time with
// -ldflags "-X github.com/ship8t/ship8t/internal/version.stampedVersion=<version>".
var stampedVersion = ""

// Current returns the version to show users. It prefers the stamped version,
// then the module version from "go install", then "dev".
func Current() string {
	if stampedVersion != "" {
		return stampedVersion
	}
	if version := moduleVersion(); version != "" {
		return version
	}
	return "dev"
}

// moduleVersion returns the version recorded by "go install ...@<version>".
func moduleVersion() string {
	buildInfo, ok := debug.ReadBuildInfo()
	if !ok || buildInfo.Main.Version == "(devel)" {
		return ""
	}
	return buildInfo.Main.Version
}
