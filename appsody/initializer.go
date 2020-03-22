package appsody

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Initializer of functions using the appsody binary.
type Initializer struct {
	// Verbose logging flag.
	Verbose bool
}

// NewInitializer creates an instance of an appsody-backed initializer.
func NewInitializer() *Initializer {
	return &Initializer{}
}

// Initialize a new funciton of the given name, of the given language, at the given path.
func (n *Initializer) Initialize(name, language, path string) error {
	// Check for the appsody binary explicitly so that we can return
	// an extra-friendly error message.
	_, err := exec.LookPath("appsody")
	if err != nil {
		return errors.New("please install 'appsody'")
	}

	// Appsody does not support domain names as the project name
	// (ex: www.example.com), and has extremely strict naming requirements
	// (only lower case letters, numbers and dashes).  So for now replace
	// any dots with dashes.
	name = strings.ReplaceAll(name, ".", "-")

	// set up the command, specifying a sanitized project name and connecting
	// standard output and error.
	cmd := exec.Command("appsody", "init", stackName(language), "--project-name", name)
	cmd.Dir = path

	// If verbose logging is enabled, echo appsody's chatty stdout.
	if n.Verbose {
		cmd.Stdout = os.Stdout
	}

	// Capture stderr for echoing on failure.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Run the command, echoing captured stderr as well ass the cmd internal error.
	err = cmd.Run()
	if err != nil {
		// TODO: sanitize stderr from appsody, or submit a PR to remove duplicates etc.
		err = errors.New(fmt.Sprintf("%v. %v", string(stderr.Bytes()), err.Error()))
	}
	return err
}

func stackName(language string) string {
	return "boson/go"
}
