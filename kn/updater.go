package kn

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/boson-project/faas/k8s"
)

// Updater implemented using the kn binary.
type Updater struct {
	// Verbose logging.
	Verbose bool
}

// NewUpdater creates an instance of the kubectl-based deployer.
func NewUpdater() *Updater {
	return &Updater{}
}

// Update the named service with the new image.
func (d *Updater) Update(name, image string) (err error) {
	// assert kubectl
	if _, err = exec.LookPath("kn"); err != nil {
		return errors.New("please install 'kn'")
	}

	// Convert the project name proper (a valid domain) to how it is being
	// represented in kubernetes:  as a domain label (RFC1035)
	// for use as the service's deployed name.
	project, err := k8s.ToSubdomain(name)
	if err != nil {
		return
	}

	timestamp := fmt.Sprintf("BUILT=%v", time.Now().Format("20060102T150405"))

	// TODO: use knative client directly.
	// TODO: use tags and traffic splitting.
	cmd := exec.Command("kn", "service", "update", project, "--env", timestamp)

	// If verbose logging is enabled, echo appsody's chatty stdout.
	if d.Verbose {
		fmt.Println(cmd)
		cmd.Stdout = os.Stdout
	}

	// Capture stderr for echoing on failure.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Run the command, echoing captured stderr as well ass the cmd internal error.
	if err = cmd.Run(); err != nil {
		// TODO: sanitize stderr from appsody, or submit a PR to remove duplicates etc.
		return errors.New(fmt.Sprintf("%v. %v", string(stderr.Bytes()), err.Error()))
	}

	// TODO: explicitly pull address:
	return nil
}
