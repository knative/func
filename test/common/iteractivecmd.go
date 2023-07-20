package common

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

type TestInteractiveCmd struct {
	TestCmd *TestExecCmd
	T       *testing.T

	// Sleep interval before first command input
	StartSleepInterval time.Duration

	// Sleep interval between each subcommand
	CommandSleepInterval time.Duration

	// Sleep interval after last command completion.
	// Required time to give to process to complete before EOF
	CompletionSleepInterval time.Duration

	// Timeout before kill the cmd in case of some failure
	CompletionTimeout time.Duration
}

func NewTestShellInteractiveCmd(t *testing.T) *TestInteractiveCmd {
	testShell := NewKnFuncShellCli(t)
	return &TestInteractiveCmd{
		TestCmd:                 testShell,
		T:                       t,
		StartSleepInterval:      time.Second * 2,
		CommandSleepInterval:    time.Second * 1,
		CompletionSleepInterval: time.Second * 2,
		CompletionTimeout:       time.Second * 15,
	}
}

// PrepareRun creates a go function used to start kn func (binary) that requires user interaction such as `func config command`
func (f *TestInteractiveCmd) PrepareRun(funcCommand ...string) func(args ...string) TestExecCmdResult {

	return func(userInput ...string) TestExecCmdResult {

		// Prepare Command args
		finalArgs := f.TestCmd.BinaryArgs
		if finalArgs == nil {
			finalArgs = funcCommand
		} else if funcCommand != nil {
			finalArgs = append(finalArgs, funcCommand...)
		}
		if f.TestCmd.ShouldDumpCmdLine {
			f.T.Log(f.TestCmd.Binary, strings.Join(finalArgs, " "))
		}

		// Prepare terminal emulator
		c, _, err := vt10x.NewVT10XConsole()
		if err != nil {
			f.T.Fatal(err)
		}
		defer c.Close()

		// Prepare and start command on terminal emulator
		var stdout bytes.Buffer

		cmd := exec.Command(f.TestCmd.Binary, finalArgs...)
		cmd.Stdin = c.Tty()
		cmd.Stdout = io.MultiWriter(c.Tty(), &stdout)
		cmd.Stderr = io.MultiWriter(c.Tty(), &stdout)
		if f.TestCmd.SourceDir != "" {
			cmd.Dir = f.TestCmd.SourceDir
		}
		cmd.Env = append(os.Environ(), f.TestCmd.Env...)

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
		for i, subcmd := range userInput {
			if i == 0 {
				time.Sleep(f.StartSleepInterval)
			} else {
				time.Sleep(f.CommandSleepInterval)
			}
			_, err = c.Send(subcmd)
			if err != nil {
				f.T.Logf("error sending user input comand to console: %v\n", err)
			}
		}
		time.Sleep(f.CompletionSleepInterval)
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
		case <-time.After(f.CompletionTimeout):
			err = cmd.Process.Kill()
			if err != nil {
				fmt.Printf("error killing process after timeout: %v\n", err)
			} else {
				err = fmt.Errorf("timeout occurred")
			}
		}

		// Collect results
		result := TestExecCmdResult{
			Out:   stdout.String(),
			Error: err,
		}
		if err == nil && f.TestCmd.ShouldDumpOnSuccess {
			f.T.Log(result.Out)
		}
		if err != nil {
			f.T.Log(result.Out)
			f.T.Log(err.Error())
		}
		return result
	}
}
