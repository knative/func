package appsody_test

import (
	"fmt"
	"os"
	"testing"
)

// TestRun ensures that the local appsody binary runs a stack as expected.
func TestRun(t *testing.T) {
	// Abort if not explicitly enabled.
	if !*enableTestAppsodyIntegration {
		fmt.Fprintln(os.Stdout, "Skipping tests which require 'appsody' binary.  enable with --enable-test-appsody-initializer")
		t.Skip()
	}

	// Testdata Function Service
	//
	// The directory has been pre-populated with a runnable base function by running
	// init and committing the result, such that this test is not dependent on a
	// functioning creation step.  This code will need to be updated for any
	// backwards incompatible changes to the templated base go func.

	// Run the function

	// TODO: in a separate goroutine, submit an HTTP or CloudEvent request to the
	// running function, and confirm expected OK response, then close down the
	// running function by interrupting/canceling run.  This may requre an update
	// to the runner to run the command with a cancelable context, and cancel on
	// normal signals as well as a signal specific to these tests such as SIGUSR1.

	/*
		runner := appsody.NewRunner()
		err := runner.Run("./testdata/example.com/runnable")
		if err != nil {
			t.Fatal(err)
		}
		..
		// submit cloud event
		...
		// send signal os.SIGUSR1
	*/

}
