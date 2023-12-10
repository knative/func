package cmd

import (
	"fmt"
	"testing"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mock"
)

func TestDebugger(t *testing.T) {
	// CD into a new tmp directory
	var (
		// root     = fromTempDirectory(t)
		root     = "/home/dfridric/test-environment/ns-te"
		ns       = "twoer"
		deployer = mock.NewDeployer()
		builder  = mock.NewBuilder()
	)

	// Create a new default function in Go
	_, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	cmd := NewDeployCmd(NewTestClient(fn.WithDeployer(deployer), fn.WithBuilder(builder)))
	// cmd := NewBuildCmd(NewTestClient())
	// cmd.SetArgs([]string{"--builder=pack", "--verbose"}) // Or whatever you're testing
	cmd.SetArgs([]string{"--verbose", fmt.Sprintf("--namespace=%s", ns)})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}
