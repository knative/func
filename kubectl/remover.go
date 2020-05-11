package kubectl

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"

	faasclient "github.com/boson-project/faas/client"
	"github.com/boson-project/faas/k8s"
)

// Remover implemented using the kubectl binary.
type Remover struct {
	// Verbose logging.
	Verbose bool
}

// NewRemover creates an instance of the kubectl-based deployer.
func NewRemover() *Remover {
	return &Remover{}
}

// Remove the named service.
// Name is expected
func (d *Remover) Remove(name string) (err error) {
	// assert kubectl
	if _, err = exec.LookPath("kubectl"); err != nil {
		err = errors.New("please install 'kubectl'")
		return
	}

	// Convert the project name proper (a valid domain) to how it is being
	// represented:  as a kubernetes and docker valid name (RFC1035 label)
	serviceName, err := k8s.ToSubdomain(name)
	if err != nil {
		return
	}

	// Command to run
	cmd := exec.Command("kubectl", "delete", "kservice", serviceName, "--namespace", faasclient.DefaultNamespace)

	// If verbose logging is enabled, echo appsody's chatty stdout.
	if d.Verbose {
		fmt.Println(cmd)
		cmd.Stdout = os.Stdout
	}

	// Capture stderr for echoing on failure.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Run the command, echoing captured stderr as well as the cmd internal error.
	err = cmd.Run()
	if err != nil {
		err = errors.New(fmt.Sprintf("%v. %v", string(stderr.Bytes()), err.Error()))
	}
	return
}
