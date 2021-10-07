package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

type TestShellCmdRunner struct {

	// binary to invoke
	// Example: "func", "kn", "kubectl", "/usr/bin/sh"
	Binary string

	// Binary args to append before actual args. Examples:
	//  when 'kn' binary binaryArgs should be ["func"]
	BinaryArgs []string

	// Run commands from Dir
	SourceDir string

	// Indicates shell should dump command line args during execution
	ShouldDumpCmdLine bool

	// Indicates shell should dump
	ShouldDumpOnSuccess bool

	// Environment variable to be used with the command
	Env []string

	// Boolean
	T *testing.T
}

// TestShellCmdResult stored command result
type TestShellCmdResult struct {
	Stdout string
	Stderr string
	Error  error
}

func (r TestShellCmdResult) Dump(t *testing.T) {
	if r.Stdout != "" {
		t.Log(r.Stdout)
	}
	if r.Stderr != "" {
		t.Log(r.Stderr)
	}
}

func NewKnFuncShellCli(t *testing.T) *TestShellCmdRunner {
	knfunc := TestShellCmdRunner{}
	knfunc.T = t

	if IsUseKnFunc() {
		knfunc.Binary = "kn"
		knfunc.BinaryArgs = []string{"func"}
	} else {
		knfunc.Binary = GetFuncBinaryPath()
		if knfunc.Binary == "" {
			t.Log("'func' binary not defined. Please set E2E_FUNC_BIN_PATH environment variable prior to run this test")
			t.FailNow()
		}
	}
	cmd := knfunc.Exec()
	if cmd.Error != nil {
		t.FailNow()
	}
	knfunc.ShouldDumpCmdLine = true
	return &knfunc
}

func (f *TestShellCmdRunner) WithEnv(envKey string, envValue string) *TestShellCmdRunner {
	env := envKey + "=" + envValue
	f.Env = append(f.Env, env)
	return f
}

func (f *TestShellCmdRunner) FromDir(dir string) *TestShellCmdRunner {
	f.SourceDir = dir
	return f
}

// Exec invokes go exec library and runs a shell command combining the binary args with args from method signature
func (f *TestShellCmdRunner) Exec(args ...string) TestShellCmdResult {
	finalArgs := f.BinaryArgs
	if finalArgs == nil {
		finalArgs = args
	} else if args != nil {
		finalArgs = append(finalArgs, args...)
	}

	if f.ShouldDumpCmdLine {
		f.T.Log(f.Binary, strings.Join(finalArgs, " "))
	}

	var stderr bytes.Buffer
	var stdout bytes.Buffer

	cmd := exec.Command(f.Binary, finalArgs...)
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	if f.SourceDir != "" {
		cmd.Dir = f.SourceDir
	}
	cmd.Env = append(os.Environ(), f.Env...)
	err := cmd.Run()

	result := TestShellCmdResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
		Error:  err,
	}

	if err == nil && f.ShouldDumpOnSuccess {
		f.T.Log(result.Stdout)
	}
	if err != nil {
		f.T.Log(err.Error())
		f.T.Log(result.Stderr)
	}

	return result
}
