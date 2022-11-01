package cmd

import (
	"path/filepath"
	"testing"

	fn "knative.dev/func"
	"knative.dev/func/mock"
)

// TestDescribe_ByName ensures that describing a function by name invokes
// the describer appropriately.
func TestDescribe_ByName(t *testing.T) {
	var (
		testname  = "testname"
		describer = mock.NewDescriber()
	)

	describer.DescribeFn = func(n string) (fn.Instance, error) {
		if n != testname {
			t.Fatalf("expected describe name '%v', got '%v'", testname, n)
		}
		return fn.Instance{}, nil
	}

	cmd := NewDescribeCmd(NewClientFactory(func() *fn.Client {
		return fn.New(fn.WithDescriber(describer))
	}))
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
	root := fromTempDirectory(t)

	err := fn.New().Create(fn.Function{
		Name:     "testname",
		Runtime:  "go",
		Registry: TestRegistry,
		Root:     root,
	})
	if err != nil {
		t.Fatal(err)
	}

	describer := mock.NewDescriber()
	describer.DescribeFn = func(n string) (i fn.Instance, err error) {
		if n != "testname" {
			t.Fatalf("expected describer to receive name 'testname', got '%v'", n)
		}
		return
	}
	cmd := NewDescribeCmd(NewClientFactory(func() *fn.Client {
		return fn.New(fn.WithDescriber(describer))
	}))
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

// TestDescribe_NameAndPathExclusivity ensures that providing both a name
// and a path will generate an error.
func TestDescribe_NameAndPathExclusivity(t *testing.T) {
	d := mock.NewDescriber()
	cmd := NewDescribeCmd(NewClientFactory(func() *fn.Client {
		return fn.New(fn.WithDescriber(d))
	}))
	cmd.SetArgs([]string{"-p", "./testpath", "testname"})
	if err := cmd.Execute(); err == nil {
		// TODO(lkingland): use a typed error
		t.Fatalf("expected error on conflicting flags not received")
	}
	if d.DescribeInvoked {
		t.Fatal("describer was invoked when conflicting flags were provided")
	}
}

// TestDescribe_Namespace ensures that the namespace provided to the client
// for use when describing a function is set
// 1. The flag /env variable if provided
// 2. The namespace of the function at path if provided
// 3. The user's current active namespace
func TestDescribe_Namespace(t *testing.T) {
	root := fromTempDirectory(t)

	client := fn.New(fn.WithDescriber(mock.NewDescriber()))

	// Ensure that the default is "default" when no context can be identified
	t.Setenv("KUBECONFIG", filepath.Join(cwd(), "nonexistent"))
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	cmd := NewDescribeCmd(func(cc ClientConfig, _ ...fn.Option) (*fn.Client, func()) {
		if cc.Namespace != "default" {
			t.Fatalf("expected 'default', got '%v'", cc.Namespace)
		}
		return client, func() {}
	})
	cmd.SetArgs([]string{"somefunc"}) // by name such that no f need be created
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Ensure the extant function's namespace is used
	f := fn.Function{
		Root:    root,
		Runtime: "go",
		Deploy: fn.DeploySpec{
			Namespace: "deployed",
		},
	}
	if err := client.Create(f); err != nil {
		t.Fatal(err)
	}
	cmd = NewDescribeCmd(func(cc ClientConfig, _ ...fn.Option) (*fn.Client, func()) {
		if cc.Namespace != "deployed" {
			t.Fatalf("expected 'deployed', got '%v'", cc.Namespace)
		}
		return client, func() {}
	})
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Ensure an explicit namespace is plumbed through
	cmd = NewDescribeCmd(func(cc ClientConfig, _ ...fn.Option) (*fn.Client, func()) {
		if cc.Namespace != "ns" {
			t.Fatalf("expected 'ns', got '%v'", cc.Namespace)
		}
		return client, func() {}
	})
	cmd.SetArgs([]string{"--namespace", "ns"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

}
