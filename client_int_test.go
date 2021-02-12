// +build integration

package function_test

import (
	boson "github.com/boson-project/func"
	"github.com/boson-project/func/knative"
	"testing"
)

/*
 NOTE:  Running integration tests locally requires a configured test cluster.
        Test failures may require manual removal of dangling resources.

 ## Integration Cluster

 These integration tests require a properly configured cluster,
 such as that which is setup and configured in CI (see .github/workflows).
 A local KinD cluster can be started via:
   ./hack/allocate.sh && ./hack/configure.sh

 ## Integration Testing

 These tests can be run via the make target:
   make integration
  or manually by specifying the tag
   go test -v -tags integration ./...

 ## Teardown and Cleanup

 Tests should clean up after themselves.  In the event of failures, one may
 need to manually remove files:
   rm -rf ./testdata/example.com
 The test cluster is not automatically removed, as it can be reused.  To remove:
   ./hack/delete.sh
*/

const DefaultNamespace = "func"

func TestList(t *testing.T) {
	verbose := true

	// Assemble
	lister, err := knative.NewLister(DefaultNamespace)
	if err != nil {
		t.Fatal(err)
	}
	client := boson.New(
		boson.WithLister(lister),
		boson.WithVerbose(verbose))

	// Act
	names, err := client.List()
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if len(names) != 0 {
		t.Fatalf("Expected no Functions, got %v", names)
	}
}
