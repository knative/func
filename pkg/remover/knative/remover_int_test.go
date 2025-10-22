//go:build integration

package knative_test

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/rand"
	"knative.dev/func/pkg/describer"
	k8sdescriber "knative.dev/func/pkg/describer/k8s"
	knativedescriber "knative.dev/func/pkg/describer/knative"
	"knative.dev/func/pkg/lister"
	k8slister "knative.dev/func/pkg/lister/k8s"
	knativelister "knative.dev/func/pkg/lister/knative"
	"knative.dev/func/pkg/remover"
	k8sremover "knative.dev/func/pkg/remover/k8s"
	knativeremover "knative.dev/func/pkg/remover/knative"

	knativedeployer "knative.dev/func/pkg/deployer/knative"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/oci"
	fntest "knative.dev/func/pkg/testing"
)

func TestInt_Remove(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	name := "func-int-knative-remove-" + rand.String(5)
	root := t.TempDir()
	ns := fntest.Namespace(t, ctx)

	t.Cleanup(cancel)

	client := fn.New(
		fn.WithBuilder(oci.NewBuilder("", false)),
		fn.WithPusher(oci.NewPusher(true, true, true)),
		fn.WithDeployer(knativedeployer.NewDeployer(knativedeployer.WithDeployerVerbose(true))),
		fn.WithDescriber(describer.NewMultiDescriber(false, knativedescriber.NewDescriber(false), k8sdescriber.NewDescriber(false))),
		fn.WithLister(lister.NewLister(false, knativelister.NewGetter(false), k8slister.NewGetter(false))),
		fn.WithRemover(remover.NewMultiRemover(false, knativeremover.NewRemover(false), k8sremover.NewRemover(false))),
	)

	f, err := client.Init(fn.Function{
		Root:      root,
		Name:      name,
		Runtime:   "go",
		Namespace: ns,
		Registry:  fntest.Registry(),
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
	list, err := client.List(ctx, "")
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
	list, err = client.List(ctx, "")
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
