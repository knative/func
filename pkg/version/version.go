package version

import "github.com/Masterminds/semver/v3"

var Vers, Kver, Hash string

// DefaultVers is the fallback version used when no build-time version was
// injected (e.g. source builds that bypass the Makefile).
const DefaultVers = "v0.0.0+source"

// Get returns the parsed semver for this binary.  When no build-time version
// was injected via ldflags, DefaultVers is used.  If the injected string is
// unparseable, DefaultVers is used as a safe fallback.
// String() returns a clean semver without the leading "v" (e.g. "0.0.0+source"),
// suitable for machine-readable consumers such as the MCP server.
// Original() round-trips the injected string verbatim, preserving the leading
// "v" preferred by human-readable output.
func Get() *semver.Version {
	s := Vers
	if s == "" {
		s = DefaultVers
	}
	v, err := semver.NewVersion(s) // permissive: accepts leading 'v'
	if err != nil {
		v, _ = semver.NewVersion(DefaultVers)
	}
	return v
}
