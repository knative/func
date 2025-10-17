//go:build integration

package knative_test

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/rand"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/knative"
	"knative.dev/func/pkg/oci"
)

func TestInt_List(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	name := "func-int-knative-list-" + rand.String(5)
	root := t.TempDir()
	ns := namespace(t, ctx)

	t.Cleanup(cancel)

	client := fn.New(
		fn.WithBuilder(oci.NewBuilder("", false)),
		fn.WithPusher(oci.NewPusher(true, true, true)),
		fn.WithDeployer(knative.NewDeployer(knative.WithDeployerVerbose(true))),
		fn.WithDescriber(knative.NewDescriber(false)),
		fn.WithLister(knative.NewLister(false)),
		fn.WithRemover(knative.NewRemover(false)),
	)

	f, err := client.Init(fn.Function{
		Root:      root,
		Name:      name,
		Runtime:   "go",
		Namespace: ns,
		Registry:  registry(),
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
	list, err := client.List(ctx, "")
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
