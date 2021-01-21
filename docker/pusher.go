package docker

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	bosonFunc "github.com/boson-project/func"
)

// Pusher of images from local to remote registry.
type Pusher struct {
	// Verbose logging.
	Verbose bool
}

// NewPusher creates an instance of a docker-based image pusher.
func NewPusher() *Pusher {
	return &Pusher{}
}

// Push the image of the Function.
func (n *Pusher) Push(f bosonFunc.Function) (digest string, err error) {
	// Check for the docker binary explicitly so that we can return
	// an extra-friendly error message.
	_, err = exec.LookPath("docker")
	if err != nil {
		err = errors.New("please install 'docker'")
		return
	}

	if f.Image == "" {
		return "", errors.New("Function has no associated image.  Has it been built?")
	}

	// set up the command, specifying a sanitized project name and connecting
	// standard output and error.
	cmd := exec.Command("docker", "push", f.Image)

	// Capture the command output in the buffer
	var output bytes.Buffer

	// If verbose logging is enabled, echo chatty stdout.
	if n.Verbose {
		fmt.Println(cmd)
		cmd.Stdout = io.MultiWriter(&output, os.Stdout)
	} else {
		cmd.Stdout = &output
	}

	// Capture stderr for echoing on failure.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Run the command, echoing captured stderr as well ass the cmd internal error.
	err = cmd.Run()
	if err != nil {
		// TODO: sanitize stderr from docker?
		err = fmt.Errorf("%v. %v", stderr.String(), err.Error())
	}

	digest = parseDigest(output.String())

	return
}

// parseDigest tries to parse the last line from the output, which holds the pushed image digest
// The last line should look like this:
// latest: digest: sha256:a278a91112d17f8bde6b5f802a3317c7c752cf88078dae6f4b5a0784deb81782 size: 2613
func parseDigest(output string) string {

	// get last line from the output
	lines := strings.Split(output, "\n")
	lastline := lines[len(lines)-2]

	// find the start index of the "digest" section
	shaIndex := strings.Index(lastline, "sha256")
	if shaIndex == -1 {
		return ""
	}
	subStr := lastline[shaIndex:]

	// find the end index of the "digest" section
	endIndex := strings.Index(subStr, " ")
	if endIndex == -1 {
		return ""
	}
	return subStr[:endIndex]
}
