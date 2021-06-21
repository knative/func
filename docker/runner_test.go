// +build !integration

package docker_test

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"

	fn "github.com/boson-project/func"
	"github.com/boson-project/func/docker"
)

// Docker Run Integraiton Test
// This is an integraiton test meant to be manually run in order to confirm proper funcitoning of the docker runner.
// It requires that the funciton already be built.

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
	// TODO: This test is too tricky, as it requires the related image be
	// already built.  Build the funciton prior to running?

	runner := docker.NewRunner()
	runner.Verbose = true
	if err = runner.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	/* TODO
	// submit cloud event
	// send os.SIGUSR1 in leay of SIGTERM
	*/

}
