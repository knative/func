//go:build e2e

package e2e

import (
	"bytes"
	"fmt"
	"net"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"knative.dev/func/test/oncluster"
	"knative.dev/func/test/testhttp"

	common "knative.dev/func/test/common"
)

// TestFunctionExtendedFlow will run a comprehensive path of func commands an end user may perform such as
// > create > build > run > (curl)+invoke > deploy > describe > list > (curl)+invoke > deploy (new revision) > (curl) > delete > list
func TestFunctionExtendedFlow(t *testing.T) {

	var funcName = "extended-test"
	var funcPath = filepath.Join(t.TempDir(), funcName)

	knFunc := common.NewKnFuncShellCli(t)
	knFunc.ShouldDumpOnSuccess = false

	// ---------------------------
	// Func Create Test
	// ---------------------------
	knFunc.Exec("create", "--language", "node", funcPath)

	// From here on, all commands will be executed from the func project path
	knFunc.SourceDir = funcPath

	// ---------------------------
	// Func Build Test
	// ---------------------------
	knFunc.Exec("build", "--registry", common.GetRegistry())

	// ---------------------------
	// Func Run Test
	// ---------------------------
	knFuncTerm1 := common.NewKnFuncShellCli(t)
	knFuncTerm1.ShouldDumpOnSuccess = false
	knFuncTerm2 := common.NewKnFuncShellCli(t)
	knFuncTerm2.ShouldDumpOnSuccess = true

	portChannel := make(chan string)
	go func() {
		t.Log("----Checking for listening port")
		// set the function that will be executed while "kn func run" is executed
		knFuncTerm1.OnWaitCallback = func(stdout *bytes.Buffer) {
			t.Log("-----Executing OnWaitCallback")
			funcPort, attempts := "", 0
			for funcPort == "" && attempts < 10 {
				t.Logf("----Function Output:\n%v\n", stdout.String())
				findPort := func(exp string, msg string) (port string) {
					matches := regexp.MustCompile(exp).FindStringSubmatch(msg)
					if len(matches) > 1 {
						port = matches[1]
					}
					return
				}
				funcPort = findPort("Running on host port (.*)", stdout.String())
				if funcPort == "" {
					funcPort = findPort("Function started on port (.*)", stdout.String())
				}
				attempts++
				if funcPort == "" {
					time.Sleep(200 * time.Millisecond)
				}
			}
			// can proceed
			portChannel <- funcPort
		}
		knFuncTerm1.Exec("run", "--verbose", "--path", funcPath)
	}()

	knFuncRunCompleted := false
	knFuncRunProcessFinalizer := func() {
		if knFuncRunCompleted == false {
			knFuncTerm1.ExecCmd.Process.Signal(syscall.SIGTERM)
			knFuncRunCompleted = true
		}
	}
	defer knFuncRunProcessFinalizer()

	// Get running port (usually 8080) from func output. We can use it for test http.
	funcPort := <-portChannel
	assert.Assert(t, funcPort != "", "Unable to retrieve local port allocated for function")

	// Wait Port to be available
	net.DialTimeout("tcp", ":"+funcPort, 3*time.Second)
	time.Sleep(time.Second)

	// GET Function HTTP Endpoint
	_, bodyResp := testhttp.TestGet(t, "http://localhost:"+funcPort+"?message=local")
	assert.Assert(t, strings.Contains(bodyResp, `{"message":"local"}`), "function response does not contain expected body.")

	// ---------------------------
	// Func Invoke Locally Test
	// ---------------------------
	result := knFuncTerm2.Exec("invoke", "--path", funcPath)
	assert.Assert(t, strings.Contains(result.Out, `{"message":"Hello World"}`), "function response does not contain expected body.")

	// Stop "kn func run" execution
	knFuncRunProcessFinalizer()
	time.Sleep(2 * time.Second)

	// ---------------------------
	// Func Deploy Test
	// ---------------------------
	knFuncDelete := func(t *testing.T) {
		knFunc.Exec("delete", funcName)
		listResult := knFunc.Exec("list")
		assert.Assert(t, strings.Contains(listResult.Out, funcName) == false, "Function is listed as deployed after delete")
	}

	result = knFunc.Exec("deploy", "--build=false")
	firstRevisionName, functionUrl := common.WaitForFunctionReady(t, funcName)
	defer knFuncDelete(t)

	wasDeployed := regexp.MustCompile("✅ Function [a-z]* in namespace .* at URL: \n   http.*").MatchString(result.Out)
	assert.Assert(t, wasDeployed, "Function was not deployed")

	urlFromDeploy := ""
	matches := regexp.MustCompile("URL: \n   (http.*)").FindStringSubmatch(result.Out)
	if len(matches) > 1 {
		urlFromDeploy = matches[1]
	}
	assert.Assert(t, urlFromDeploy != "", "URL not returned on deploy output")

	// ---------------------------
	// Func Describe Test
	// ---------------------------
	urlFromDescribe := ""
	result = knFunc.Exec("describe", "--output", "plain")
	matches = regexp.MustCompile("Route (http.*)").FindStringSubmatch(result.Out)
	if len(matches) > 1 {
		urlFromDescribe = matches[1]
	}
	assert.Assert(t, urlFromDescribe != "", "URL not returned on info output")
	assert.Assert(t, urlFromDescribe == urlFromDeploy, fmt.Sprintf("URL from 'func info' [%s] does not match URL from 'func deploy' [%s]", urlFromDescribe, urlFromDeploy))
	assert.Assert(t, urlFromDescribe == functionUrl, "URL does not match knative service URL")

	// ---------------------------
	// Func List Test
	// ---------------------------
	result = knFunc.Exec("list")
	assert.Assert(t, strings.Contains(result.Out, funcName), "deployed function is not listed")

	// ---------------------------
	// Invoke Remote Test
	// ---------------------------
	result = knFunc.Exec("invoke")
	assert.Assert(t, strings.Contains(result.Out, `{"message":"Hello World"}`), "function response does not contain expected body.")

	// GET Function HTTP Endpoint
	_, bodyResp = testhttp.TestGet(t, functionUrl+"?message=remote")
	assert.Assert(t, strings.Contains(bodyResp, `{"message":"remote"}`), "function response does not contain expected body.")

	// ---------------------------
	// Deploy new revision
	// ---------------------------
	oncluster.WriteNewSimpleIndexJS(t, funcPath, "NEW_REVISION")
	knFunc.Exec("deploy")

	common.WaitForNewRevisionReady(t, firstRevisionName, funcName)

	// ---------------------------
	// Call New Revision
	// ---------------------------
	_, bodyResp = testhttp.TestGet(t, functionUrl)
	assert.Assert(t, strings.Contains(bodyResp, "NEW_REVISION"), "function new revision does not contain expected body.")

	// Delete Test (on defer)
}
