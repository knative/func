package appsody

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
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

	// Appsody does not support domain names as the project name
	// (ex: www.example.com), and has extremely strict naming requirements
	// (only lower case letters, numbers and dashes).  So for now replace
	// any dots with dashes.
	name = strings.ReplaceAll(name, ".", "-")

	// Fully qualified image name.  Ex quay.io/user/www-example-com:20200102T1234
	timestamp := time.Now().Format("20060102T150405")
	image = fmt.Sprintf("%v/%v/%v:%v", n.registry, n.namespace, name, timestamp)

	// set up the command, specifying a sanitized project name and connecting
	// standard output and error.
	cmd := exec.Command("appsody", "build", "--knative", "-t", image)
	cmd.Dir = path

	fmt.Println("***** RUNNING *****")
	fmt.Println(cmd)

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
	return
}
