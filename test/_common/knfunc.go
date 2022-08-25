package common

import (
	"testing"

	e2e "knative.dev/kn-plugin-func/test/_e2e"
)

func NewKnFuncShellCli(t *testing.T) *TestExecCmd {
	knFunc := TestExecCmd{}
	knFunc.T = t

	if e2e.IsUseKnFunc() {
		knFunc.Binary = "kn"
		knFunc.BinaryArgs = []string{"func"}
	} else {
		knFunc.Binary = e2e.GetFuncBinaryPath()
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
	return &knFunc
}
