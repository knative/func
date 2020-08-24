package appsody

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/boson-project/faas/k8s"
)

// NameMappings are short-name to repository full name mappings,
// enabling shorthand `faas create go` rather than `faas create go-ce-functions`
var StackShortNames = map[string]string{
	"go":      "go-ce-functions",
	"node":    "node-ce-functions",
	"quarkus": "quarkus-ce-functions",
}

// Initializer of Functions using the appsody binary.
type Initializer struct {
	// Verbose logging flag.
	Verbose bool
}

// NewInitializer creates an instance of an appsody-backed initializer.
func NewInitializer() *Initializer {
	return &Initializer{}
}

// Initialize a new Function of the given name, of the given runtime, at the given path.
func (n *Initializer) Initialize(name, runtime, path string) error {
	// Check for the appsody binary explicitly so that we can return
	// an extra-friendly error message.
	_, err := exec.LookPath("appsody")
	if err != nil {
		return errors.New("please install 'appsody'")
	}

	// Appsody does not support domain names as the project name
	// (ex: www.example.com), and has extremely strict naming requirements
	// (subdomains per rfc 1035).  So let's just assume its name must be a valid domain, and
	// encode it as a 1035 domain by doubling down on hyphens.
	project, err := k8s.ToSubdomain(name)
	if err != nil {
		return err
	}

	// Dereference stack short name.  ex. "go" -> "go-ce-functions"
	stackName, ok := StackShortNames[runtime]
	if !ok {
		runtimes := []string{}
		for k := range StackShortNames {
			runtimes = append(runtimes, k)
		}

		return fmt.Errorf("Unrecognized runtime '%v'.  Please choose one: %v.", runtime, strings.Join(runtimes, ", "))
	}

	// set up the command, specifying a sanitized project name and connecting
	// standard output and error.
	cmd := exec.Command("appsody", "init", "boson/"+stackName, "--project-name", project)
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
		err = fmt.Errorf("%v. %v", stderr.String(), err.Error())
	}
	return err
}
