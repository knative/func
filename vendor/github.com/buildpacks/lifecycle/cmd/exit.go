package cmd

import (
	"fmt"
	"os"
	"strings"
)

const (
	// lifecycle errors not specific to any phase: 1-99
	CodeFailed = 1 // CodeFailed indicates generic lifecycle error
	// 2: reserved
	CodeInvalidArgs = 3
	// 4: CodeInvalidEnv
	// 5: CodeNotFound
	// 9: CodeFailedUpdate

	// API errors
	CodeIncompatiblePlatformAPI  = 11
	CodeIncompatibleBuildpackAPI = 12
)

type LifecycleExitError int

const (
	FailedDetect           LifecycleExitError = iota
	FailedDetectWithErrors                    // no buildpacks detected
	DetectError                               // no buildpacks detected and at least one errored
	AnalyzeError                              // generic analyze error
	RestoreError                              // generic restore error
	FailedBuildWithErrors                     // buildpack error during /bin/build
	BuildError                                // generic build error
	ExportError                               // generic export error
	RebaseError                               // generic rebase error
	LaunchError                               // generic launch error
)

type Platform interface {
	API() string
	CodeFor(errType LifecycleExitError) int
}

type ErrorFail struct {
	Err    error
	Code   int
	Action []string
}

func (e *ErrorFail) Error() string {
	message := "failed to " + strings.Join(e.Action, " ")
	if e.Err == nil {
		return message
	}
	return fmt.Sprintf("%s: %s", message, e.Err)
}

func FailCode(code int, action ...string) *ErrorFail {
	return FailErrCode(nil, code, action...)
}

func FailErr(err error, action ...string) *ErrorFail {
	code := CodeFailed
	if err, ok := err.(*ErrorFail); ok {
		code = err.Code
	}
	return FailErrCode(err, code, action...)
}

func FailErrCode(err error, code int, action ...string) *ErrorFail {
	return &ErrorFail{Err: err, Code: code, Action: action}
}

func Exit(err error) {
	if err == nil {
		os.Exit(0)
	}
	DefaultLogger.Errorf("%s\n", err)
	if err, ok := err.(*ErrorFail); ok {
		os.Exit(err.Code)
	}
	os.Exit(CodeFailed)
}

func ExitWithVersion() {
	DefaultLogger.Infof(buildVersion())
	os.Exit(0)
}
