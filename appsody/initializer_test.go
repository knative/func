package appsody_test

import (
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/lkingland/faas/appsody"
)

// Enabling Appsody Binary Integration Tests
//
// The appsody package constitutes an integration between the core client logic
// and the concrete implementation of an Initializer using the appsody binary.
// These tests are therefore disabled by default for local development but are
// enabled during deploy for e2e tests etc.
//
// For development of this package in particualr, the flag will need to be
// either manually set, or ones IDE set to add the flag such that the tests run.
var enableTestAppsodyIntegration = flag.Bool("enable-test-appsody-integration", false, "Enable tests requiring local appsody binary.")

// TestInitialize ensures that the local appsody binary initializes a stack as expected.
func TestInitialize(t *testing.T) {
	// Abort if not explicitly enabled.
	if !*enableTestAppsodyIntegration {
		fmt.Fprintln(os.Stdout, "Skipping tests which require 'appsody' binary.  enable with --enable-test-appsody-integration")
		t.Skip()
	}

	// Create the directory in which the service function will be initialized,
	// removing it on test completion.
	if err := os.Mkdir("testdata/example.com/www", 0700); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata/example.com/www")

	// Instantiate a new appsody-based initializer
	initializer := appsody.NewInitializer()
	initializer.Verbose = true
	if err := initializer.Initialize("testfunc", "go", "testdata/example.com/www"); err != nil {
		t.Fatal(err)
	}

	// TODO: verify the stack appears as expected?
}

// TestInvalidInput ensures that invalid input to the
func TestInvalidInput(t *testing.T) {
	// TODO: dots in names are converted
	//       missing input (empty strings)
	//       invalid paths
}
