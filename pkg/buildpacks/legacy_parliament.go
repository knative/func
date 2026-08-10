package buildpacks

// LEGACY PYTHON: pack support for old parliament functions — skip scaffolding and
// build from the user's Procfile + requirements.txt with cloudevents pinned <2.
// Delete this file on parliament sunset.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	fn "knative.dev/func/pkg/functions"
)

const (
	// pip constraints file at the build root, referenced via PIP_CONSTRAINT.
	legacyConstraintsFile = "constraints.txt"
	legacyCloudeventsPin  = "cloudevents<2"
)

const legacyDeprecationWarning = "Warning: this function uses the legacy parliament Python signature (def main(context)), " +
	"which is deprecated and will be removed in a future release.\n" +
	"func builds it the old way, from your Procfile and requirements.txt, pinning cloudevents below 2.0 (required by parliament).\n" +
	"Migrate to the current Python function layout, documented at https://github.com/knative/func/blob/main/docs/function-templates/python.md"

// legacyScaffold replaces pack scaffolding: warn, clear stale scaffolding, and
// write the cloudevents<2 constraint (leaving the user's Procfile/requirements).
func legacyScaffold(verbose bool, f fn.Function) error {
	fmt.Fprintln(os.Stderr, legacyDeprecationWarning)
	if verbose {
		fmt.Printf("Legacy parliament function detected; skipping python scaffolding for '%v'\n", f.Root)
	}
	// clear stale scaffolding from a prior non-legacy build
	if err := os.RemoveAll(filepath.Join(f.Root, defaultPath)); err != nil {
		return fmt.Errorf("cannot clean stale scaffolding directory: %w", err)
	}
	if err := writeCloudeventsConstraint(f.Root); err != nil {
		return fmt.Errorf("unable to write legacy parliament constraints file: %w", err)
	}
	return nil
}

// cloudeventsConstrainedRe matches a (non-comment) cloudevents requirement line
// carrying a version specifier. A bare "cloudevents" line or a comment merely
// mentioning it does not constrain anything.
var cloudeventsConstrainedRe = regexp.MustCompile(`(?im)^[ \t]*cloudevents[ \t]*(\[[^\]]*\])?[ \t]*[<>=!~]`)

// writeCloudeventsConstraint appends cloudevents<2 to constraints.txt at root,
// preserving any existing user constraints and skipping only when the user
// already carries a cloudevents version constraint.
// Must live at root (not .func/) — the remote PVC upload excludes .func/.
func writeCloudeventsConstraint(root string) error {
	path := filepath.Join(root, legacyConstraintsFile)
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if cloudeventsConstrainedRe.Match(existing) {
		return nil
	}
	content := string(existing)
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += legacyCloudeventsPin + "\n"
	return os.WriteFile(path, []byte(content), 0644)
}

// legacyPackEnv points pip at the constraints file.
func legacyPackEnv(env map[string]string) {
	env["PIP_CONSTRAINT"] = legacyConstraintsFile
}
