package appsody

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// NameMappings are short-name to repository full name mappings,
// enabling shorthand `faas create go` rather than `faas create go-ce-functions`
var stackShortNames = map[string]string{
	"go":   "go-ce-functions",
	"js":   "node-ce-functions",
	"java": "quarkus-ce-functions",
}

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

	// Dereference stack short name
	stackName, ok := stackShortNames[language]
	if !ok {
		languages := []string{}
		for k, _ := range stackShortNames {
			languages = append(languages, k)
		}

		return errors.New(fmt.Sprintf("Unrecognized lanugage '%v'.  Please choose one: %v.", language, strings.Join(languages, ", ")))
	}

	// set up the command, specifying a sanitized project name and connecting
	// standard output and error.
	cmd := exec.Command("appsody", "init", "boson/"+stackName, "--project-name", name)
	cmd.Dir = path

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
	return err
}
