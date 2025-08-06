//go:build e2e && linux

package e2e

import (
	"bytes"
	"errors"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"knative.dev/func/test/testhttp"

	common "knative.dev/func/test/common"
)

// TestFunctionRunWithoutContainer tests the func runs on host without container (golang funcs only)
// In other words, it tests `func run --container=false`
func TestFunctionRunWithoutContainer(t *testing.T) {

	var funcName = "func-no-container"
	var funcPath = filepath.Join(t.TempDir(), funcName)

	knFuncTerm1 := common.NewKnFuncShellCli(t)
	knFuncTerm1.Exec("create", "--language", "go", "--template", "http", funcPath)

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
			for funcPort == "" && attempts < 30 { // 15 secs
				t.Logf("----Function Output:\n%v", stdout.String())
				matches := regexp.MustCompile("Running on host port (.*)").FindStringSubmatch(stdout.String())
				attempts++
				if len(matches) > 1 {
					funcPort = matches[1]
				} else {
					time.Sleep(500 * time.Millisecond)
				}
			}
			// can proceed
			portChannel <- funcPort
		}

		// Run without container (scaffolding)
		knFuncTerm1.Exec("run", "--container=false", "--verbose", "--path", funcPath, "--registry", common.GetRegistry())
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

	// Assert Function endpoint responds
	_, bodyResp := testhttp.TestGet(t, "http://localhost:"+funcPort+"?message=run-on-host")
	assert.Assert(t, strings.Contains(bodyResp, `GET /?message=run-on-host`), "function response does not contain expected body.")

	// Assert Func were not built
	_, err := os.Stat(filepath.Join(funcPath, ".func", "built"))
	assert.Assert(t, errors.Is(err, os.ErrNotExist), "File .func/built exists")

}
