package testing

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/rand"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/oci"
	. "knative.dev/func/pkg/testing"
	. "knative.dev/func/pkg/testing/k8s"
)

func TestInt_Remove(t *testing.T, remover fn.Remover, deployer fn.Deployer, describer fn.Describer, lister fn.Lister, deployerName string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	name := "func-int-knative-remove-" + rand.String(5)
	root := t.TempDir()
	ns := Namespace(t, ctx)

	t.Cleanup(cancel)

	client := fn.New(
		fn.WithBuilder(oci.NewBuilder("", false)),
		fn.WithPusher(oci.NewPusher(true, true, true)),
		fn.WithDeployer(deployer),
		fn.WithRemovers(remover),
		fn.WithDescribers(describer),
		fn.WithListers(lister),
	)

	f, err := client.Init(fn.Function{
		Root:      root,
		Name:      name,
		Runtime:   "go",
		Namespace: ns,
		Registry:  Registry(),
		Deploy: fn.DeploySpec{
			Deployer: deployerName,
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

	// Wait for function to be ready
	_, err = client.Describe(ctx, "", "", f)
	if err != nil {
		t.Fatal(err)
	}

	// Verify with list
	list, err := client.List(ctx, ns)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range list {
		if item.Name == f.Name {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("function %s not found in list", f.Name)
	}

	// Remove it
	if err := client.Remove(ctx, "", "", f, true); err != nil {
		t.Logf("error removing Function: %v", err)
	}

	// Verify it is no longer listed
	list, err = client.List(ctx, ns)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, item := range list {
		if item.Name == f.Name {
			found = true
			break
		}
	}
	if found {
		t.Errorf("function %s was not removed", f.Name)
	}

	// Remove

}
