package knative

import (
	"bytes"
	"fmt"
	"io"
	"os"

	commands "knative.dev/client/pkg/kn/commands"
	"knative.dev/client/pkg/kn/core"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/k8s"
)

// TODO: Use knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1
// NewForConfig gives you the client, and then you can do
// client.Services("ns").Get("name")

type Deployer struct {
	// Namespace with which to override that set on the default configuration (such as the ~/.kube/config).
	// If left blank, deployment will commence to the configured namespace.
	Namespace string
	// Verbose logging enablement flag.
	Verbose bool
}

func NewDeployer() *Deployer {
	return &Deployer{}
}

func (d *Deployer) Deploy(f faas.Function) (err error) {
	// k8s does not support service names with dots.  so encode it such that
	// www.my-domain,com -> www-my--domain-com
	encodedName, err := k8s.ToK8sAllowedName(f.Name)
	if err != nil {
		return
	}

	// Capture output in a buffer if verbose is not enabled for output on error.
	var output io.Writer
	if d.Verbose {
		output = os.Stdout
	} else {
		output = &bytes.Buffer{}
	}

	params := commands.KnParams{}
	params.Initialize()
	params.Output = output
	c := core.NewKnCommand(params)
	c.SetOut(output)
	args := []string{
		"service", "create", encodedName,
		"--image", f.Image,
		"--env", "VERBOSE=true",
		"--label", "bosonFunction=true",
	}
	if d.Namespace != "" {
		args = append(args, "--namespace", d.Namespace)
	}
	c.SetArgs(args)
	err = c.Execute()
	if err != nil {
		if !d.Verbose {
			err = fmt.Errorf("failed to deploy the service: %v.\nStdOut: %s", err, output.(*bytes.Buffer).String())
		} else {
			err = fmt.Errorf("failed to deploy the service: %v", err)
		}
		return
	}
	// TODO: use the KN service client noted above, such that we can return the
	// final path/route of the final deployed Function.  While it can be assumed
	// due to being deterministic, new users would be aided by having it echoed.
	return
}
