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

func TestInt_List(t *testing.T, lister fn.Lister, deployer fn.Deployer, describer fn.Describer, remover fn.Remover, deployerName string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	name := "func-int-knative-list-" + rand.String(5)
	root := t.TempDir()
	ns := fnk8stest.Namespace(t, ctx)

	t.Cleanup(cancel)

	client := fn.New(
		fn.WithBuilder(oci.NewBuilder("", false)),
		fn.WithPusher(oci.NewPusher(true, true, true)),
		fn.WithDeployer(deployer),
		fn.WithListers(lister),
		fn.WithDescribers(describer),
		fn.WithRemovers(remover),
	)

	f, err := client.Init(fn.Function{
		Root:      root,
		Name:      name,
		Runtime:   "go",
		Namespace: ns,
		Registry:  fntest.Registry(),
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

	// Verify with list
	list, err := client.List(ctx, ns)
	if err != nil {
		t.Fatal(err)
	}

	// Should find at least our function (may have others in namespace)
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

}
