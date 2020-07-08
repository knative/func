package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	root.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version",
	Run:   version,
}

func version(cmd *cobra.Command, args []string) {
	fmt.Println(verboseVersion())
}

// Populated at build time by `make build`, plumbed through
// main using SetMeta()
var (
	date string // datestamp
	vers string // verstionof git commit or `tip`
	hash string // git hash built from
)

// SetMeta from the build process, used for verbose version tagging.
func SetMeta(buildTimestamp, commitVersionTag, commitHash string) {
	date = buildTimestamp
	vers = commitVersionTag
	hash = commitHash
}

func verboseVersion() string {
	// If building from source (i.e. from `go install` or `go build` directly,
	// simply print 'v0.0.0-source`, a semver-valid version indicating no version
	// number.  Otherwise print the verbose version populated during `make build`.
	if vers == "" { // not statically populatd
		return "v0.0.0-source"
	}
	return fmt.Sprintf("%s-%s-%s", date, vers, hash)
}
