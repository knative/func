//go:build integration

package testing

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/rand"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/oci"
	fntest "knative.dev/func/pkg/testing"
	fnk8stest "knative.dev/func/pkg/testing/k8s"
)

func DescribeIntegrationTest(t *testing.T, describer fn.Describer, deployer fn.Deployer, remover fn.Remover, deployType string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	name := "func-int-knative-describe-" + rand.String(5)
	root := t.TempDir()
	ns := fnk8stest.Namespace(t, ctx)

	t.Cleanup(cancel)

	client := fn.New(
		fn.WithBuilder(oci.NewBuilder("", false)),
		fn.WithPusher(oci.NewPusher(true, true, true)),
		fn.WithDescribers(describer),
		fn.WithDeployer(deployer),
		fn.WithRemovers(remover),
	)

	f, err := client.Init(fn.Function{
		Root:      root,
		Name:      name,
		Runtime:   "go",
		Namespace: ns,
		Registry:  fntest.Registry(),
		Deploy: fn.DeploySpec{
			DeployType: deployType,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Build
	f, err = client.Build(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	// Push
	f, _, err = client.Push(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	// Deploy
	f, err = client.Deploy(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		err := client.Remove(ctx, "", "", f, true)
		if err != nil {
			t.Logf("error removing Function: %v", err)
		}
	})

	// Wait for function to be ready
	_, err = client.Describe(ctx, "", "", f)
	if err != nil {
		t.Fatal(err)
	}

	// Describe
	desc, err := client.Describe(ctx, "", "", f)
	if err != nil {
		t.Fatal(err)
	}

	if desc.Name != f.Name {
		t.Fatalf("expected name %q, got %q", f.Name, desc.Name)

	}
	if desc.Namespace != ns {
		t.Fatalf("expected namespace %q, got %q", ns, desc.Namespace)
	}
}
