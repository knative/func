package cmd

import (
	"fmt"
	"strings"

	"github.com/ory/viper"
	"github.com/spf13/cobra"
)

// metadata about the build process/binary etc.
// Not populated if building from source with go build.
// Set by the `make` targets.
var version = Version{}

// SetMeta is called by `main` with any provided build metadata
func SetMeta(date, vers, hash string) {
	version.Date = date // build timestamp
	version.Vers = vers // version tag
	version.Hash = hash // git commit hash
}

func init() {
	root.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:        "version",
	Short:      "Show the version",
	Long:       `Show the version

Use the --verbose option to include the build date stamp and commit hash"
`,
	SuggestFor: []string{"vers", "verison"},
	Run:        runVersion,
}

func runVersion(cmd *cobra.Command, args []string) {
	// update version with the value of the (global) flag 'verbose'
	version.Verbose = viper.GetBool("verbose")

	// version is the metadata, serialized.
	fmt.Println(version)
}

// versionMetadata is set by the main package.
// When compiled from source, they remain the zero value.
// When compiled via `make`, they are initialized to the noted values.
type Version struct {
	// Date of compilation
	Date string
	// Version tag of the git commit, or 'tip' if no tag.
	Vers string
	// Hash of the currently active git commit on build.
	Hash string
	// Verbose printing enabled for the string representation.
	Verbose bool
}

func (v Version) String() string {
	// If 'vers' is not a semver already, then the binary was built either
	// from an untagged git commit (set semver to v0.0.0), or was built
	// directly from source (set semver to v0.0.0-source).
	if strings.HasPrefix(v.Vers, "v") {
		// Was built via make with a tagged commit
		if v.Verbose {
			return fmt.Sprintf("%s-%s-%s", v.Vers, v.Hash, v.Date)
		} else {
			return v.Vers
		}
	} else if v.Vers == "tip" {
		// Was built via make from an untagged commit
		v.Vers = "v0.0.0"
		if v.Verbose {
			return fmt.Sprintf("%s-%s-%s", v.Vers, v.Hash, v.Date)
		} else {
			return v.Vers
		}
	} else {
		// Was likely built from source
		v.Vers = "v0.0.0"
		v.Hash = "source"
		if v.Verbose {
			return fmt.Sprintf("%s-%s", v.Vers, v.Hash)
		} else {
			return v.Vers
		}
	}
}
