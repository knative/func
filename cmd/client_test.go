package cmd

import (
	"context"
	"testing"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mock"
)

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

	// Trigger an invocation of the mocks by running the associated client
	// methods which depend on them
	err := client.Remove(context.Background(), "myfunc", "myns", fn.Function{}, true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Describe(context.Background(), "myfunc", "myns", fn.Function{})
	if err != nil {
		t.Fatal(err)
	}

	// Ensure the first set of options, held on the factory (the mock remover)
	// is invoked.
	if !remover.RemoveInvoked {
		t.Fatalf("factory (outer) options not carried through to final client instance")
	}
	// Ensure the second set of options, provided when constructing the client
	// using the factory, are ignored.
	if describer.DescribeInvoked {
		t.Fatalf("test client factory should ignore options when invoked.")
	}

	// This ensures that the NewTestClient function, when provided a set
	// of optional implementations (mocks) will override any which are set
	// by commands, allowing tests to "force" a command to use the mocked
	// implementations.
}
