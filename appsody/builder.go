package appsody

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// Builder of images from function source using appsody.
type Builder struct {
	// Verbose logging flag.
	Verbose bool

	registry  string // registry domain (docker.io, quay.io, etc.)
	namespace string // namespace (username, org name, etc.)
}

// NewBuilder creates an instance of an appsody-backed image builder.
func NewBuilder(registry, namespace string) *Builder {
	return &Builder{
		registry:  registry,
		namespace: namespace}
}

// Build an image from the funciton source at path.
func (n *Builder) Build(name, path string) (image string, err error) {
	// Check for the appsody binary explicitly so that we can return
	// an extra-friendly error message.
	_, err = exec.LookPath("appsody")
	if err != nil {
		err = errors.New("please install 'appsody'")
		return
	}

	// Fully qualified image name.  Ex quay.io/user/www-example-com:20200102T1234
	// timestamp := time.Now().Format("20060102T150405")
	// image = fmt.Sprintf("%v/%v/%v:%v", n.registry, n.namespace, name, timestamp)

	// Simple image name, which uses :latest
	image = fmt.Sprintf("%v/%v/%v", n.registry, n.namespace, name)

	// set up the command, specifying a sanitized project name and connecting
	// standard output and error.
	cmd := exec.Command("appsody", "build", "-t", image)
	cmd.Dir = path

	// If verbose logging is enabled, echo appsody's chatty stdout.
	if n.Verbose {
		fmt.Println(cmd)
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
	return
}
