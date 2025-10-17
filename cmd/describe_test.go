package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mock"
	. "knative.dev/func/pkg/testing"
)

// TestDescribe_Default ensures that running describe when there is no
// function in the given directory fails correctly.
func TestDescribe_Default(t *testing.T) {
	_ = FromTempDirectory(t)
	describer := mock.NewDescriber()

	cmd := NewDescribeCmd(NewTestClient(fn.WithDescriber(describer)))
	cmd.SetArgs([]string{})
	err := cmd.Execute()

	if err == nil {
		t.Fatal("describing a nonexistent function should error")
	}
	if !strings.Contains(err.Error(), "function not found at this path and no name provided") {
		t.Fatalf("Unexpected error text returned: %v", err)
	}
	if describer.DescribeInvoked {
		t.Fatal("Describer incorrectly invoked")
	}
}

// TestDescribe_Undeployed ensures that describing a function which exists,
// but has not been deployed, does not error but rather delegates to the
// deployer which will presumably describe it as being !deployed (See deployer
// test suite)
func TestDescribe_Undeployed(t *testing.T) {
	root := FromTempDirectory(t)

	client := fn.New()
	_, err := client.Init(fn.Function{
		Name:     "testfunc",
		Runtime:  "go",
		Registry: TestRegistry,
		Root:     root,
	})
	if err != nil {
		t.Fatal(err)
	}

	describer := mock.NewDescriber()

	cmd := NewDescribeCmd(NewTestClient(fn.WithDescriber(describer)))
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if !describer.DescribeInvoked {
		t.Fatal("Describer should have been invoked for any initialized function")
	}
}

// TestDescribe_ByName ensures that describing a function by name invokes
// the describer appropriately.
func TestDescribe_ByName(t *testing.T) {
	var (
		testname  = "testname"
		describer = mock.NewDescriber()
	)

	describer.DescribeFn = func(_ context.Context, name, namespace string) (fn.Instance, error) {
		if name != testname {
			t.Fatalf("expected describe name '%v', got '%v'", testname, name)
		}
		return fn.Instance{}, nil
	}

	cmd := NewDescribeCmd(NewTestClient(fn.WithDescriber(describer)))
	cmd.SetArgs([]string{testname})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if !describer.DescribeInvoked {
		t.Fatal("Describer not invoked")
	}
}

// TestDescribe_ByProject ensures that describing the currently active project
// (func created in the current working directory) invokes the describer with
// its name correctly.
func TestDescribe_ByProject(t *testing.T) {
	root := FromTempDirectory(t)
	expected := "testname"

	_, err := fn.New().Init(fn.Function{
		Name:     expected,
		Runtime:  "go",
		Registry: TestRegistry,
		Root:     root,
	})
	if err != nil {
		t.Fatal(err)
	}

	describer := mock.NewDescriber()
	describer.DescribeFn = func(_ context.Context, name, namespace string) (i fn.Instance, err error) {
		if name != expected {
			t.Fatalf("expected describer to receive name %q, got %q", expected, name)
		}
		return
	}
	cmd := NewDescribeCmd(NewTestClient(fn.WithDescriber(describer)))
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

// TestDescribe_NameAndPathExclusivity ensures that providing both a name
// and a path will generate an error.
func TestDescribe_NameAndPathExclusivity(t *testing.T) {
	d := mock.NewDescriber()
	cmd := NewDescribeCmd(NewTestClient(fn.WithDescriber(d)))
	cmd.SetArgs([]string{"-p", "./testpath", "testname"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error on conflicting flags not received")
	} else if !errors.Is(err, ErrNameAndPathConflict) {
		t.Fatalf("expected ErrNameAndPathExclusivity, got %v", err)
	}
	if d.DescribeInvoked {
		t.Fatal("describer was invoked when conflicting flags were provided")
	}
}
