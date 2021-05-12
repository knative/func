package e2e

import (
	"github.com/boson-project/func/test/cli"
	"github.com/boson-project/func/test/e2e/utils"
	"regexp"
	"time"
)

// WaitFunctionToBecomeReady Waits a function to become ready. Timeout in 60 sec
func WaitFunctionToBecomeReady(funcCli *cli.TestShellCli, functionName string) bool {
	ch := make(chan bool, 1)
	defer close(ch)
	go func() {
		attempts := 1
		expr, _ := regexp.Compile("\n" + functionName + " .*True")
		out := funcCli.RunSilent("list").Stdout
		for !expr.Match([]byte(out)) {
			if (attempts > 12) {
				ch <- false
				return
			}
			time.Sleep(5 * time.Second)
			out = funcCli.Run("list").Stdout
			attempts++
		}
		ch <- true
	}()
	select {
	case r := <- ch:
		return r
	case <-time.After( 60 * time.Second) :
		return false
	}
}

// GetLatestReadyFunctionRevision Retrieve latest ready revision
func GetLatestReadyFunctionRevision(kubectlCli *cli.TestShellCli, functionName string) string {
	cmd := kubectlCli.Run("get", "ksvc", functionName)
	if cmd.HasError() {
		return ""
	}
	// NAME       URL                                             LATESTCREATED    LATESTREADY      READY   REASON
	// nodefunc   http://nodefunc.default.192.168.39.188.nip.io   nodefunc-00003   nodefunc-00003   True

	funcInfo := utils.StringExtractLineFieldsMatching(cmd.Stdout, 0, functionName)
	if len(funcInfo) <= 3 {
		return ""
	}
	// Get Revision Name (LATESTREADY column)
	return funcInfo[3]
}