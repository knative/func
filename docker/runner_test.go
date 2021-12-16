//go:build !integration
// +build !integration

package docker_test

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/docker"
)

// Docker Run Integraiton Test
// This is an integraiton test meant to be manually run in order to confirm proper functioning of the docker runner.
// It requires that the function already be built.

var enableTestDocker = flag.Bool("enable-test-docker", false, "Enable tests requiring local docker.")

func TestDockerRun(t *testing.T) {
	// Skip this test unless explicitly enabled, as it is more of
	// an integration test.
	if !*enableTestDocker {
		fmt.Fprintln(os.Stdout, "Skipping docker integration test for 'run'.  Enable with --enable-test-docker")
		t.Skip()
	}

	f, err := fn.NewFunction("testdata/example.com/runnable")
	if err != nil {
		t.Fatal(err)
	}

	// NOTE: test requries that the image be built already.

	runner := docker.NewRunner()
	runner.Verbose = true
	errCh := make(chan error, 1)
	if _, _, err = runner.Run(context.Background(), f, errCh); err != nil {
		t.Fatal(err)
	}
	/* TODO
	// submit cloud event
	// send os.SIGUSR1 in leay of SIGTERM
	*/

}

func TestDockerRunImagelessError(t *testing.T) {
	runner := docker.NewRunner()
	f := fn.NewFunctionWith(fn.Function{})

	errCh := make(chan error, 1)
	_, _, err := runner.Run(context.Background(), f, errCh)
	expectedErrorMessage := "Function has no associated Image. Has it been built? Using the --build flag will build the image if it hasn't been built yet"
	if err == nil || err.Error() != expectedErrorMessage {
		t.Fatalf("The expected error message is \"%v\" but got instead %v", expectedErrorMessage, err)
	}
}
