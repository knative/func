package e2e

import (
	"regexp"
	"strings"
	"testing"
)

// Deploy runs `func deploy' command for a given test project. It collects the URL from output
// and store on test project, so it can be used later by any another test
func Deploy(t *testing.T, knFunc *TestShellCmdRunner, project *FunctionTestProject) {

	var result TestShellCmdResult
	if project.IsBuilt {
		result = knFunc.Exec("deploy", "--path", project.ProjectPath, "--registry", GetRegistry(), "--build=false")
	} else {
		result = knFunc.Exec("deploy", "--path", project.ProjectPath, "--registry", GetRegistry())
	}
	if result.Error != nil {
		t.Fail()
	}

	// Remove some unwanted control chars to make easy to parse the result
	cleanStdout := CleanOutput(result.Stdout)

	// Deploy command would output the URL, so user can use it to call the functions. Example:
	// "Function [deployed|updated] at URL: http://nodefunc.default.192.168.39.188.nip.io"
	// Here we extract the URL and store on project setting so that can be used later
	// to validate actual function responsiveness.
	wasDeployed := regexp.MustCompile("✅ Function [a-z]* in namespace .* at URL: \n   http.*").MatchString(cleanStdout)
	if !wasDeployed {
		t.Fatal("Function was not deployed")
	}

	urlMatch := regexp.MustCompile("(URL: \n   http.*)").FindString(cleanStdout)
	if urlMatch == "" {
		t.Fatal("URL not returned on output")
	}

	project.FunctionURL = strings.TrimSpace(strings.Split(urlMatch, "\n")[1])
	project.IsDeployed = true

}
