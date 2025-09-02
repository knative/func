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

func TestInt_Labels(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	name := "func-int-knative-describe-" + rand.String(5)
	root := t.TempDir()
	ns := namespace(t, ctx)

	t.Cleanup(cancel)

	client := fn.New(
		fn.WithBuilder(oci.NewBuilder("", false)),
		fn.WithPusher(oci.NewPusher(true, true, true)),
		fn.WithDeployer(knative.NewDeployer(knative.WithDeployerVerbose(true))),
		fn.WithDescriber(knative.NewDescriber(false)),
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
