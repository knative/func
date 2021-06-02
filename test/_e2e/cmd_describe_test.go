package e2e

import (
	"regexp"
	"testing"
)

// Describe runs `func describe' command basic test.
func Describe(t *testing.T, knFunc *TestShellCmdRunner, project *FunctionTestProject)  {

	result := knFunc.Exec("describe", "--path", project.ProjectPath, "--output", "plain")
	if result.Error != nil {
		t.Fail()
	}

	// Here we do a basic check in the output.
	// In case we have the route stored (i.e. by deploy command tested earlier)
	// we compare just to verify they match
	// otherwise we take advantage and capture the route from the output
	routeFromDescribe := ""

	matches := regexp.MustCompile("Route (http.*)").FindStringSubmatch(result.Stdout)
	if len(matches) > 1 {
		routeFromDescribe = matches[1]
	}
	if routeFromDescribe == "" {
		t.Fatal("Function Route not present on output")
	}
	if project.FunctionURL != "" && project.FunctionURL != routeFromDescribe {
		t.Fatalf("Expected Route %v but found %v", project.FunctionURL, routeFromDescribe)
	}
	project.FunctionURL = routeFromDescribe

}
