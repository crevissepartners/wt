package buildinfo

import "runtime/debug"

const (
	// ModulePath is the canonical Go module path used for release installs.
	ModulePath = "github.com/crevissepartners/wt"
)

// Version can be overridden at build time via -ldflags.
var Version = "0.10.12" // x-release-please-version

// EffectiveVersion returns a user-facing version string.
//
// For module installs (e.g. go install ...@latest), prefer the tagged module
// version from build info. Local builds fall back to the release-please-managed
// source version, which can still be overridden by -ldflags.
func EffectiveVersion() string {
	info, ok := debug.ReadBuildInfo()
	if ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	if Version != "" {
		return Version
	}
	return "dev"
}
