package cmd

import (
	"context"
	"testing"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mock"
)

const namespace = "func"

// Test_NewTestClient ensures that the convenience method for
// constructing a mocked client for testing properly considers options:
// options provided to the factory constructor are considered exaustive,
// such that the test can force the user of the factory to use specific mocks.
// In other words, options provided when invoking the factory (such as by
// a command implementation) are ignored.
func Test_NewTestClient(t *testing.T) {
	var (
		remover   = mock.NewRemover()
		describer = mock.NewDescriber()
	)

	// Factory constructor options which should be used when invoking later
	clientFn := NewTestClient(fn.WithRemover(remover))
	// Factory should ignore options provided when invoking
	client, _ := clientFn(ClientConfig{}, fn.WithDescriber(describer))

	// Trigger an invocation of the mocks
	err := client.Remove(context.Background(), fn.Function{Name: "test", Deploy: fn.DeploySpec{Namespace: namespace}}, true)
	if err != nil {
		t.Fatal(err)
	}
	f, err := fn.NewFunction("")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Describe(context.Background(), "test", f)
	if err != nil {
		t.Fatal(err)
	}

	// Ensure the first set of options, held on the factory, were used
	if !remover.RemoveInvoked {
		t.Fatalf("factory (outer) options not carried through to final client instance")
	}
	// Ensure the second set of options, provided when constructing the
	// client using the factory, were ignored
	if describer.DescribeInvoked {
		t.Fatalf("test client factory should ignore options when invoked.")
	}
}
