package e2e

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/hinshun/vt10x"
)

type TestShellInteractiveCmdRunner struct {
	TestShell *TestShellCmdRunner
	T         *testing.T

	// Sleep interval between each subcommand
	commandSleepInterval time.Duration

	// Sleep interval after last command completion.
	// Required time to give to process to complete before EOF
	completionSleepInterval time.Duration

	// Timeout before kill the cmd in case of some failure
	completionTimeout time.Duration
}

func NewTestShellInteractiveCmdRunner(t *testing.T) *TestShellInteractiveCmdRunner {
	testShell := NewKnFuncShellCli(t)
	return &TestShellInteractiveCmdRunner{
		TestShell:               testShell,
		T:                       t,
		commandSleepInterval:    time.Second * 1,
		completionSleepInterval: time.Second * 2,
		completionTimeout:       time.Second * 15,
	}
}

// Prepare creates a go function used to start kn func (binary) that requires user interaction such as `func config command`
func (f *TestShellInteractiveCmdRunner) PrepareRun(funcCommand ...string) func(args ...string) TestShellCmdResult {

	return func(userInput ...string) TestShellCmdResult {

		// Prepare Command args
		finalArgs := f.TestShell.BinaryArgs
		if finalArgs == nil {
			finalArgs = funcCommand
		} else if funcCommand != nil {
			finalArgs = append(finalArgs, funcCommand...)
		}
		if f.TestShell.ShouldDumpCmdLine {
			f.T.Log(f.TestShell.Binary, strings.Join(finalArgs, " "))
		}

		// Prepare terminal emulator
		c, _, err := vt10x.NewVT10XConsole()
		if err != nil {
			f.T.Fatal(err)
		}
		defer c.Close()

		// Prepare and start command on terminal emulator
		var stderr bytes.Buffer
		var stdout bytes.Buffer

		cmd := exec.Command(f.TestShell.Binary, finalArgs...)
		cmd.Stdin = c.Tty()
		cmd.Stdout = io.MultiWriter(c.Tty(), &stdout)
		cmd.Stderr = io.MultiWriter(c.Tty(), &stderr)
		if f.TestShell.SourceDir != "" {
			cmd.Dir = f.TestShell.SourceDir
		}
		cmd.Env = append(os.Environ(), f.TestShell.Env...)

		err = cmd.Start()
		if err != nil {
			f.T.Fatalf("error on start command: %v\n", err)
		}

		// Monitor kn func command completion
		doneCh := make(chan error, 1)
		go func() {
			_, err := c.ExpectEOF()
			doneCh <- err
		}()

		// Input user entries on Terminal
		for _, subcmd := range userInput {
			time.Sleep(f.commandSleepInterval)
			_, err = c.Send(subcmd)
			if err != nil {
				f.T.Logf("error sending user input comand to console: %v\n", err)
			}
		}
		time.Sleep(f.completionSleepInterval)
		err = c.Tty().Close()
		if err != nil {
			f.T.Logf("error on TTY close: %v\n", err)
		}

		// Wait Command Completion
		select {
		case err = <-doneCh:
			if err != nil {
				fmt.Printf("process completed with error: %v\n", err)
			}
		case <-time.After(f.completionTimeout):
			err = cmd.Process.Kill()
			if err != nil {
				fmt.Printf("error killing process after timeout: %v\n", err)
			} else {
				err = fmt.Errorf("timeout occurred")
			}
		}

		// Collect results
		result := TestShellCmdResult{
			Stdout: stdout.String(),
			Stderr: stderr.String(),
			Error:  err,
		}
		if err == nil && f.TestShell.ShouldDumpOnSuccess {
			f.T.Log(result.Stdout)
		}
		if err != nil {
			f.T.Log(result.Stdout)
			f.T.Log(err.Error())
			f.T.Log(result.Stderr)
		}
		return result
	}
}
