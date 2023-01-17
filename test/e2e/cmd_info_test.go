package e2e

import (
	"regexp"
	"testing"
)

// Info runs `func info' command basic test.
func Info(t *testing.T, knFunc *TestShellCmdRunner, project *FunctionTestProject) {

	result := knFunc.Exec("info", "--path", project.ProjectPath, "--output", "plain")
	if result.Error != nil {
		t.Fail()
	}

	// Here we do a basic check in the output.
	// In case we have the route stored (i.e. by deploy command tested earlier)
	// we compare just to verify they match
	// otherwise we take advantage and capture the route from the output
	routeFromInfo := ""

	matches := regexp.MustCompile("Route (http.*)").FindStringSubmatch(result.Stdout)
	if len(matches) > 1 {
		routeFromInfo = matches[1]
	}
	if routeFromInfo == "" {
		t.Fatal("Function Route not present on output")
	}
	if project.FunctionURL != "" && project.FunctionURL != routeFromInfo {
		t.Fatalf("Expected Route %v but found %v", project.FunctionURL, routeFromInfo)
	}
	project.FunctionURL = routeFromInfo

}
