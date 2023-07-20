package common

import (
	"testing"
)

func NewKnFuncShellCli(t *testing.T) *TestExecCmd {
	knFunc := TestExecCmd{}
	knFunc.T = t

	if IsUseKnFunc() {
		knFunc.Binary = "kn"
		knFunc.BinaryArgs = []string{"func"}
	} else {
		knFunc.Binary = GetFuncBinaryPath()
		if knFunc.Binary == "" {
			t.Log("'func' binary not defined. Please set E2E_FUNC_BIN_PATH environment variable prior to running tests")
			t.FailNow()
		}
	}
	cmd := knFunc.Exec()
	if cmd.Error != nil {
		t.FailNow()
	}
	knFunc.ShouldDumpCmdLine = true
	knFunc.ShouldFailOnError = true
	knFunc.OnFinishCallback = func(result *TestExecCmdResult) {
		cleanedOut := CleanOutput(result.Out)
		result.Out = cleanedOut
	}
	return &knFunc
}
