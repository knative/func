package cli

import (
	"bytes"
	"github.com/boson-project/func/test/e2e/utils"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

type TestShellCli struct {

	// func binary location (./func)
	cliBinary string
	// Run commands from Dir
	sourceDir string
	// Testing object
	t *testing.T
	// Last Command Execution Status and Results
	commandExecution *CommandExecution

}

type CommandExecution struct {
	// Command line
	CmdLine string
	// Shell Command Standard output
	Stdout string
	// Shell Command Standard Error
	Stderr string
	// Additional Error reported by os
	Error error
	// Execution Command
	execCommand *exec.Cmd
}

func (c *CommandExecution) Dump(t *testing.T) {
	if c.Stdout != "" {
		t.Log(c.Stdout)
	}
	if c.Stderr != "" {
		t.Log(c.Stderr)
	}
	if c.Error != nil {
		t.Log(c.Error.Error())
	}
}

func (c *CommandExecution) HasError() bool {
	if c.Error != nil || c.Stderr != "" {
		return true
	}
	return false
}

func (c *CommandExecution) ErrorDetails() string {
	if c.Error != nil {
		return c.Error.Error()
	}
	if c.Stderr != "" {
		return c.Stderr
	}
	return ""
}


func NewFuncCli(t *testing.T) *TestShellCli {
	f := TestShellCli{ t : t}
	f.cliBinary = os.Getenv("BOSON_FUNC_BIN")
	if f.cliBinary == "" {
		t.Log("'func' binary not defined. Please set BOSON_FUNC_BIN envrionment variable prior to run this test")
		t.FailNow()
	}
	_, err := os.Stat(f.cliBinary)
	if (os.IsNotExist(err)) {
		t.Logf("'func' binary file not found (%s).", f.cliBinary)
		t.FailNow()
	}
	return &f
}

func NewKubectlCli(t *testing.T) *TestShellCli {
	kubectl := TestShellCli{ t : t}
	kubectl.cliBinary = "kubectl"
	resp := kubectl.RunSilent("version")
	if resp.Stdout == "" {
		t.Log("'kubectl' not found")
		t.FailNow()
	}
	return &kubectl
}

func NewDockerCli(t *testing.T) *TestShellCli {
	dockercli := TestShellCli{ t : t}
	supportedList := []string {"docker", "podman"}
	for _, supported := range supportedList {
		dockercli.cliBinary = supported
		resp := dockercli.RunSilent("version")
		if resp.Stderr == "" {
			t.Log(utils.StringExtractLineMatching(resp.Stdout,"Version"))
			break
		}
	}
	if dockercli.cliBinary == "" {
		t.Fatal("No docker or podman cli was found")
		t.FailNow()
	}
	t.Logf("Found %v implementation\n", dockercli.cliBinary)
	return &dockercli
}


func (f *TestShellCli) T() *testing.T {
	return f.t
}

func (f *TestShellCli) CommandExecution() *CommandExecution {
	return f.commandExecution
}


func (f *TestShellCli) LogStep(stepDescription string) {
	lineSep := "----------------------------------------------------------"
	f.T().Log(lineSep)
	f.T().Log(stepDescription)
	f.T().Log(lineSep)
}


func (f *TestShellCli) dumpCommand(args ...string) {
	f.T().Log(f.cliBinary, strings.Join(args, " "))
}

// SourceDir changes the source dir location which the CLI should execute commands from
// Some 'func' commands requires t
func (f *TestShellCli) SourceDir(sourceDir string) {
	f.sourceDir = sourceDir
}

// Run runs a cli command
func (f *TestShellCli) Run(args ...string) *CommandExecution {

	result := f.RunSilent(args...)
	result.Dump(f.T())
	return result
}

// RunSilent runs a command in silent mode. Does not dump stdout and stderr to logs
func (f *TestShellCli) RunSilent(args ...string) *CommandExecution {
	f.dumpCommand(args...)
	var stderr bytes.Buffer
	var stdout bytes.Buffer

	cmd := exec.Command(f.cliBinary, args...)
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	if f.sourceDir != "" {
		cmd.Dir = f.sourceDir
	}
	cmd.Stdin = nil

	err := cmd.Run()
	result := CommandExecution{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
		Error: err,
		execCommand: cmd,
	}
	f.commandExecution = &result
	return &result
}

// RunInBackground Runs a command in background. You may call Kill command to stop it if needed
func (f *TestShellCli) RunInBackground(args ...string) *CommandExecution {
	f.dumpCommand(args...)
	var stderr bytes.Buffer
	var stdout bytes.Buffer

	cmd := exec.Command(f.cliBinary, args...)
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	if f.sourceDir != "" {
		cmd.Dir = f.sourceDir
	}
	cmd.Stdin = nil

	err := cmd.Start()
	if err != nil {
		f.T().Logf("err: %s\n", stderr.String())
		f.T().Fatal(err)
		return nil
	}

	result := CommandExecution{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
		Error: err,
		execCommand: cmd,
	}
	f.commandExecution = &result
	return &result
}

// RunWithTimeout runs cli command for a given period of time. It stop execution after a give timeout
func (f *TestShellCli) RunWithTimeout(timeout time.Duration, args ...string) *CommandExecution {
	f.dumpCommand(args...)
	cmdExecution := f.RunInBackground(args...)
	done := make(chan error, 1)
	go func() {
		done <- cmdExecution.execCommand.Wait()
	}()

	select {
	case <-time.After(timeout):
		if err := cmdExecution.execCommand.Process.Kill(); err != nil {
			f.T().Fatal("Failed to kill process.", err)
		}
		f.T().Log("Process timeout")
	case err := <-done:
		if err != nil {
			f.T().Fatalf("Process finished with error: %v", err)
		}
	}
	cmdExecution.Dump(f.T())
	return cmdExecution
}

// Kill Command started by CLI using RunInBackground
func (f *TestShellCli) Kill() {
	if err := f.commandExecution.execCommand.Process.Kill(); err != nil {
		f.T().Fatal("failed to kill: ", err)
	}
	f.commandExecution.Dump(f.T())
}
