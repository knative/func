//go:build e2e
// +build e2e

package e2e

import (
	"os"
	"regexp"
	"strings"
	"testing"

	. "knative.dev/kn-plugin-func/testing"
)

// Build runs `func build' command for a given test project.
func Build(t *testing.T, knFunc *TestShellCmdRunner, project *FunctionTestProject) {

	result := knFunc.Exec("build", "--path", project.ProjectPath, "--registry", GetRegistry())
	if result.Error != nil {
		t.Fail()
	}

	// Remove some unwanted control chars to make easy to parse the result
	cleanStdout := CleanOutput(result.Stdout)

	// Build command displays a succeed message followed by the image name derived
	wasBuilt := regexp.MustCompile("Function image built:").MatchString(cleanStdout)
	if !wasBuilt {
		t.Fatal("Function was not built")
	}
	project.IsBuilt = true

}

// TestBuild_S2I runs `func build` using the S2I builder.
func TestBuild_S2I(t *testing.T) {
	var (
		root        = "testdata/e2e/testbuild"
		bin, prefix = bin()
		cleanup     = Within(t, root) // TODO: replace with func/testing pkg
		cwd, _      = os.Getwd()      // absolute path
	)
	defer cleanup()

	run(t, bin, prefix, "create", "-v", "--language", "node", cwd)
	output := run(t, bin, prefix, "build", "-v", "--builder", "s2i")
	if !strings.Contains(output, "Function image built:") {
		t.Fatal("image not built")
	}
}
