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
	"knative.dev/func/pkg/remover"
	k8sremover "knative.dev/func/pkg/remover/k8s"
	knativeremover "knative.dev/func/pkg/remover/knative"

	knativedeployer "knative.dev/func/pkg/deployer/knative"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/oci"
	fntest "knative.dev/func/pkg/testing"
)

func TestInt_Describe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	name := "func-int-knative-describe-" + rand.String(5)
	root := t.TempDir()
	ns := fntest.Namespace(t, ctx)

	t.Cleanup(cancel)

	client := fn.New(
		fn.WithBuilder(oci.NewBuilder("", false)),
		fn.WithPusher(oci.NewPusher(true, true, true)),
		fn.WithDeployer(knativedeployer.NewDeployer(knativedeployer.WithDeployerVerbose(true))),
		fn.WithDescriber(describer.NewMultiDescriber(true, knativedescriber.NewDescriber(true), k8sdescriber.NewDescriber(true))),
		fn.WithRemover(remover.NewMultiRemover(true, knativeremover.NewRemover(true), k8sremover.NewRemover(true))),
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
