package docker

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/boson-project/faas"
)

// Runner of functions using the docker command.
type Runner struct {
	// Verbose logging flag.
	Verbose bool
}

// NewRunner creates an instance of a docker-backed runner.
func NewRunner() *Runner {
	return &Runner{}
}

// Run the function at path
func (n *Runner) Run(f faas.Function) error {
	// Check for the docker binary explicitly so that we can return
	// an extra-friendly error message.
	_, err := exec.LookPath("docker")
	if err != nil {
		return errors.New("please install 'docker'")
	}

	if f.Image == "" {
		return errors.New("Function has no associated image.  Has it been built?")
	}

	// Extra arguments to docker
	args := []string{"run", "--rm", "-t", "-p=8080:8080"}

	// If verbosity is enabled, pass along as an environment variable to the Function.
	if n.Verbose {
		args = append(args, []string{"-e VERBOSE=true"}...)
	}
	args = append(args, f.Image)

	// Set up the command with extra arguments and to run rooted at path
	cmd := exec.Command("docker", args...)
	cmd.Dir = f.Root

	// If verbose logging is enabled, echo command
	if n.Verbose {
		fmt.Println(cmd)
	}

	// We need to show the user all output, so a method to squelch
	// docker's chattiness is not immediately apparent.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the command, echoing captured stderr as well ass the cmd internal error.
	// Will run until explicitly canceled.
	// TODO: this runner is current stubbed pending an architectural discussion
	// on how closely we would like to emulate the previous funcitonality, and
	// if we can use Grid as a localhost integraiton events fabric.
	fmt.Println(cmd)
	return cmd.Run()
}
