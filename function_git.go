package function

import (
	"fmt"
	"strings"

	giturls "github.com/whilp/git-urls"
)

type Git struct {
	URL        *string `yaml:"url,omitempty"`
	Revision   *string `yaml:"revision,omitempty"`
	ContextDir *string `yaml:"contextDir,omitempty"`
}

// validateGit validates input Git option from Function config
// if mandatoryGit==true, it checks that Git options are specified (ie. build type == git)
func validateGit(git Git, mandatoryGit bool) (errors []string) {
	if git.URL != nil {
		_, err := giturls.ParseTransport(*git.URL)
		if err != nil {
			_, err = giturls.ParseScp(*git.URL)
		}
		if err != nil {
			errMsg := fmt.Sprintf("specified option \"git.url=%s\" is not valid", *git.URL)

			originalErr := err.Error()
			if !strings.HasSuffix(originalErr, "is not a valid transport") {
				errMsg = fmt.Sprintf("%s, error: %s", errMsg, originalErr)
			}
			errors = append(errors, errMsg)
		}
	} else {
		if mandatoryGit {
			errors = append(errors, "\"git.url\" must be specified for this \"build\" type")
		}
	}

	return
}
