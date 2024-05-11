package docker_test

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"

	"knative.dev/func/pkg/docker"
	fn "knative.dev/func/pkg/functions"
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

	// NOTE: test requires that the image be built already.

	runner := docker.NewRunner(true, os.Stdout, os.Stdout)
	if _, err = runner.Run(context.Background(), f, fn.DefaultStartTimeout); err != nil {
		t.Fatal(err)
	}
	/* TODO
	// submit cloud event
	// send os.SIGUSR1 in leay of SIGTERM
	*/

}

func TestDockerRunImagelessError(t *testing.T) {
	runner := docker.NewRunner(true, os.Stdout, os.Stderr)
	f := fn.NewFunctionWith(fn.Function{})

	_, err := runner.Run(context.Background(), f, fn.DefaultStartTimeout)
	// TODO: switch to typed error:
	expectedErrorMessage := "Function has no associated image. Has it been built?"
	if err == nil || err.Error() != expectedErrorMessage {
		t.Fatalf("Expected error '%v', got '%v'", expectedErrorMessage, err)
	}
}
