package e2e

import (
	"regexp"
	"testing"
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
