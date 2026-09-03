package functions

import (
	"fmt"
	"strings"

	giturls "github.com/chainguard-dev/git-urls"
)

// Source is the repository a function is built from on the cluster: the
// values of --source, --revision and --source-dir.
type Source struct {
	// URL of the repository.
	URL string `yaml:"url,omitempty"`
	// Revision to build: a branch, a tag or a commit, as git's fetch takes it.
	// Empty means the remote's default branch.
	Revision string `yaml:"revision,omitempty"`
	// Dir is the directory within the repository holding the function.
	// Empty means the repository root.
	Dir string `yaml:"dir,omitempty"`
}

// validateSource validates the source option from Function config
func validateSource(source Source) (errors []string) {
	if source.URL != "" {
		_, err := giturls.ParseTransport(source.URL)
		if err != nil {
			_, err = giturls.ParseScp(source.URL)
		}
		if err != nil {
			errMsg := fmt.Sprintf("specified option \"source.url=%s\" is not valid", source.URL)

			originalErr := err.Error()
			if !strings.HasSuffix(originalErr, "is not a valid transport") {
				errMsg = fmt.Sprintf("%s, error: %s", errMsg, originalErr)
			}
			errors = append(errors, errMsg)
		}
	}
	return
}
