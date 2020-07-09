package cmd

import (
	"fmt"
	"strings"

	"github.com/ory/viper"
	"github.com/spf13/cobra"
)

func init() {
	root.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version.  With --verbose the build date stamp and commit hash are included if available.",
	Run:   printVersion,
}

func printVersion(cmd *cobra.Command, args []string) {
	fmt.Println(version(viper.GetBool("verbose")))
}

// Populated at build time by `make build`, plumbed through
// main using SetMeta()
var (
	date string // datestamp
	vers string // version of git commit or `tip`
	hash string // git hash built from
)

// SetMeta from the build process, used for verbose version tagging.
func SetMeta(buildTimestamp, commitVersionTag, commitHash string) {
	date = buildTimestamp
	vers = commitVersionTag
	hash = commitHash
}

// return the version, optionally with verbose details as the suffix
func version(verbose bool) string {
	// If 'vers' is not a semver already, then the binary was built either
	// from an untagged git commit (set semver to v0.0.0), or was built
	// directly from source (set semver to v0.0.0-source).
	if strings.HasPrefix(vers, "v") {
		// Was built via make with a tagged commit
		if verbose {
			return fmt.Sprintf("%s-%s-%s", vers, hash, date)
		} else {
			return vers
		}
	} else if vers == "tip" {
		// Was built via make from an untagged commit
		vers = "v0.0.0"
		if verbose {
			return fmt.Sprintf("%s-%s-%s", vers, hash, date)
		} else {
			return vers
		}
	} else {
		// Was likely built from source
		vers = "v0.0.0"
		hash = "source"
		if verbose {
			return fmt.Sprintf("%s-%s", vers, hash)
		} else {
			return vers
		}
	}
}
