package s2i

// LEGACY PYTHON: s2i support for old parliament functions — skip scaffolding and
// write an assemble that installs requirements.txt with cloudevents pinned <2.
// The entrypoint stays the function's own app.sh (shipped by the 1.17 python
// templates), exactly as on 1.17. Delete this file on parliament sunset.

import (
	"fmt"
	"os"
	"path/filepath"

	fn "knative.dev/func/pkg/functions"
)

// parliament is a python-3.9-era stack; pin the s2i image to match.
const legacyPythonBuilder = "registry.access.redhat.com/ubi8/python-39"

const legacyDeprecationWarning = "Warning: this function uses the legacy parliament Python signature (def main(context)), " +
	"which is deprecated and will be removed in a future release.\n" +
	"func builds it the old way, from your Procfile and requirements.txt, pinning cloudevents below 2.0 (required by parliament).\n" +
	"Migrate to the current Python function layout, documented at https://github.com/knative/func/blob/main/docs/function-templates/python.md"

// legacyImageOverride pins ubi8/python-39, unless the user set an explicit image.
func legacyImageOverride(f fn.Function, current string) string {
	if f.IsLegacyParliament() && current == DefaultPythonBuilder {
		return legacyPythonBuilder
	}
	return current
}

// writeLegacyAssemble replaces s2i scaffolding: warn, clear appRoot, and write
// the legacy assemble to appRoot/bin/assemble (where cfg.ScriptsURL points).
func writeLegacyAssemble(verbose bool, f fn.Function, appRoot string) error {
	fmt.Fprintln(os.Stderr, legacyDeprecationWarning)
	if verbose {
		fmt.Printf("Legacy parliament function detected; writing legacy s2i assemble for '%v'\n", f.Root)
	}
	if err := os.RemoveAll(appRoot); err != nil {
		return fmt.Errorf("cannot clean scaffolding directory: %w", err)
	}
	binDir := filepath.Join(appRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("unable to create legacy scaffolding bin dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "assemble"), []byte(LegacyParliamentAssembler), 0700); err != nil {
		return fmt.Errorf("unable to write legacy assembler script: %w", err)
	}
	return nil
}

// LegacyParliamentAssembler mimics the stock ubi8 python assemble: install the
// source + requirements at $HOME, with cloudevents pinned <2. No entrypoint is
// generated — the stock run script execs the function's own app.sh, as on 1.17.
const LegacyParliamentAssembler = `#!/bin/bash
set -e

shopt -s dotglob
echo "---> (Functions/parliament) Installing application source ..."
mv /tmp/src/* "$HOME"
cd "$HOME"

# set permissions for any installed artifacts
fix-permissions /opt/app-root -P

if [[ ! -f requirements.txt ]]; then
  echo "ERROR: parliament function is missing requirements.txt (expected parliament-functions)" >&2
  exit 1
fi
echo "---> Installing dependencies ..."
pip install -r requirements.txt

# parliament-functions==0.1.0 leaves cloudevents unpinned, which resolves to
# 2.x and breaks 'import cloudevents.http'. Pin it back to the 1.x line.
echo "---> (Functions/parliament) Pinning cloudevents below 2.x ..."
pip install 'cloudevents<2'

# set permissions for any installed artifacts
fix-permissions /opt/app-root -P
`
