package version

import (
	"fmt"

	"github.com/Masterminds/semver"
)

var Vers, Kver, Hash string

// DefaultVers is the fallback version used when no build-time version was
// injected (e.g. source builds that bypass the Makefile).
const DefaultVers = "v0.0.0+source"

// defaultVersion is DefaultVers parsed once at init time.  A panic here means
// the DefaultVers constant was changed to an invalid semver string.
var defaultVersion *semver.Version

func init() {
	var err error
	defaultVersion, err = semver.NewVersion(DefaultVers)
	if err != nil {
		panic(fmt.Sprintf("version: DefaultVers constant %q is not valid semver: %v", DefaultVers, err))
	}
}

// Get returns the parsed semver for this binary.  When no build-time version
// was injected via ldflags, DefaultVers is used.  If the injected string is
// unparseable, DefaultVers is used as a safe fallback.
// String() returns a clean semver without the leading "v" (e.g. "0.0.0+source"),
// suitable for machine-readable consumers such as the MCP server.
// Original() round-trips the injected string verbatim, preserving the leading
// "v" preferred by human-readable output.
func Get() *semver.Version {
	if Vers == "" {
		return defaultVersion
	}
	v, err := semver.NewVersion(Vers) // permissive: accepts leading 'v'
	if err != nil {
		return defaultVersion
	}
	return v
}
