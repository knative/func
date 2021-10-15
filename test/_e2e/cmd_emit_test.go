// +build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEmitCommand validates func emit command
// A custom node js Function used to test 'func emit' command (see update_templates/node/events/index.js)
// An event is sent using emit with a special event source 'func:emit', expected by the custom function.
// When this source is matched, the event will get stored globally and will be returned
// as HTTP response next time it receives another event with source "e2e:check"
// A better solution could be evaluated in future.
func TestEmitCommand(t *testing.T) {

	project := FunctionTestProject{
		FunctionName: "emit-test-node",
		ProjectPath:  filepath.Join(os.TempDir(), "emit-test-node"),
		Runtime:      "node",
		Template:     "cloudevents",
	}
	knFunc := NewKnFuncShellCli(t)

	// Create new project
	Create(t, knFunc, project)
	defer project.RemoveProjectFolder()

	//knFunc.Exec("build", "-r", GetRegistry(), "-p", project.ProjectPath, "-b", "quay.io/boson/faas-nodejs-builder:v0.7.1")

	// Update the project folder with the content of update_templates/node/events/// and deploy it
	Update(t, knFunc, &project)
	defer Delete(t, knFunc, &project)

	// Issue Func Emit command
	emitMessage := "HELLO FROM EMIT"
	result := knFunc.Exec("emit", "--content-type", "text/plain", "--data", emitMessage, "--source", "func:emit", "--path", project.ProjectPath)
	if result.Error != nil {
		t.Fatal()
	}

	// Issue another event (in order to capture the event sent by emit)
	testEvent := SimpleTestEvent{
		Type:        "e2e:check",
		Source:      "e2e:check",
		ContentType: "text/plain",
		Data:        "Emit Check",
	}
	responseBody, _, err := testEvent.pushTo(project.FunctionURL, t)
	if err != nil {
		t.Fatal("error occurred while sending event", err.Error())
	}
	if responseBody == "" || !strings.Contains(responseBody, emitMessage) {
		t.Fatalf("fail to validate emit command. Expected [%v], returned [%v]", emitMessage, responseBody)
	}
}
