package common

import (
	"testing"
)

func NewShellCmd(t *testing.T, fromDirectory string) *TestExecCmd {

	shellCmd := TestExecCmd{
		Binary:              "sh",
		BinaryArgs:          []string{"-c"},
		SourceDir:           fromDirectory,
		ShouldDumpCmdLine:   true,
		ShouldDumpOnSuccess: true,
		T:                   t,
	}
	return &shellCmd
}
