package appsody

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/boson-project/faas"
)

// Runner of Functions using the appsody binary.
type Runner struct {
	// Verbose logging flag.
	Verbose bool
}

// NewRunner creates an instance of an appsody-backed runner.
func NewRunner() *Runner {
	return &Runner{}
}

// Run the Function at path
func (n *Runner) Run(f faas.Function) error {
	// Check for the appsody binary explicitly so that we can return
	// an extra-friendly error message.
	_, err := exec.LookPath("appsody")
	if err != nil {
		return errors.New("please install 'appsody'")
	}

	// Extra arguments to appsody
	args := []string{"run"}

	// If verbosity is enabled, pass along as an environment variable to the Function.
	if n.Verbose {
		args = append(args, []string{"--docker-options", "-e VERBOSE=true"}...)
	}

	// Set up the command with extra arguments and to run rooted at path
	cmd := exec.Command("appsody", args...)
	cmd.Dir = f.Root

	// If verbose logging is enabled, echo command
	if n.Verbose {
		fmt.Println(cmd)
	}

	// We need to show the user all output, so a method to squelch
	// appsody's chattiness is not immediately apparent.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the command, echoing captured stderr as well ass the cmd internal error.
	// Will run until explicitly canceled.
	// TODO:  will we need to run with context and explicitly wait for a custom
	// signal in order to play ball with tests?
	fmt.Println(cmd)
	return cmd.Run()
}
