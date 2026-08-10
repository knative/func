package tekton

// LEGACY PYTHON: asserts the remote buildpack task carries the PIP_CONSTRAINT block.

import (
	"strings"
	"testing"
)

// TestLegacyParliamentBuildpackTask asserts the rendered buildpack task carries
// the PIP_CONSTRAINT wiring (the remote path has no opts.Env to fall back on).
func TestLegacyParliamentBuildpackTask(t *testing.T) {
	task := getBuildpackTask()
	for _, want := range []string{
		"LEGACY PYTHON begin",
		`"${ENV_DIR}/PIP_CONSTRAINT"`,
		"constraints.txt",
	} {
		if !strings.Contains(task, want) {
			t.Errorf("rendered buildpack task is missing %q", want)
		}
	}
}
