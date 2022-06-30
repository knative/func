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
		result = knFunc.Exec("deploy", "--path", project.ProjectPath, "--registry", GetRegistry(), "--build=disabled")
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
	wasDeployed := regexp.MustCompile("Function [a-z]* in namespace .* at URL: \nhttp.*").MatchString(cleanStdout)
	if !wasDeployed {
		t.Fatal("Function was not deployed")
	}

	urlMatch := regexp.MustCompile("(URL: http.*)").FindString(cleanStdout)
	if urlMatch == "" {
		t.Fatal("URL not returned on output")
	}

	project.FunctionURL = strings.Split(urlMatch, " ")[1]
	project.IsDeployed = true

}

// CleanOutput Some commands, such as deploy command, spans spinner chars and cursor shifts at output which are captured and merged
// regular output messages. This functions is meant to remove these chars in order to facilitate tests assertions and data extraction from output
func CleanOutput(deployOutput string) string {
	toRemove := []string{
		"🕛 ",
		"🕐 ",
		"🕑 ",
		"🕒 ",
		"🕓 ",
		"🕔 ",
		"🕕 ",
		"🕖 ",
		"🕗 ",
		"🕘 ",
		"🕙 ",
		"🕚 ",
		"\033[1A",
		"\033[1B",
		"\033[K",
	}
	for _, c := range toRemove {
		deployOutput = strings.ReplaceAll(deployOutput, c, "")
	}
	return deployOutput
}
