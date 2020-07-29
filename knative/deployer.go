package knative

import (
	"bytes"
	"fmt"
	"github.com/boson-project/faas"
	"github.com/boson-project/faas/k8s"
	"io"
	commands "knative.dev/client/pkg/kn/commands"
	"knative.dev/client/pkg/kn/core"
	"os"
	"strings"
)

func NewDeployer() *Deployer {
	return &Deployer{Namespace: faas.DefaultNamespace}
}

type Deployer struct {
	Namespace string
	Verbose   bool
}

func (deployer *Deployer) Deploy(name, image string) (address string, err error) {

	project, err := k8s.ToSubdomain(name)
	if err != nil {
		return
	}
	nn := strings.Split(name, ".")
	if len(nn) < 3 {
		err = fmt.Errorf("invalid service name '%v', must be at least three parts.\n", name)
		return
	}

	subDomain := nn[0]
	domain := strings.Join(nn[1:], ".")

	var output io.Writer
	if deployer.Verbose {
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
		"service", "create", project,
		"--image", image,
		"--namespace", deployer.Namespace,
		"--env", "VERBOSE=true",
		"--label", fmt.Sprintf("faas.domain=%s", domain),
		"--label", "bosonFunction=true",
		"--annotation", fmt.Sprintf("faas.subdomain=%s", subDomain),
	}
	c.SetArgs(args)
	err = c.Execute()
	if err != nil {
		if !deployer.Verbose {
			err = fmt.Errorf("failed to deploy the service: %v.\nStdOut: %s", err, output.(*bytes.Buffer).String())
		} else {
			err = fmt.Errorf("failed to deploy the service: %v", err)
		}
		return
	}

	return "[cluster-address]", nil
}
