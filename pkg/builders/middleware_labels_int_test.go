//go:build integration

package builders_test

import (
	"context"
	"testing"
	"time"

	"github.com/docker/docker/client"

	"knative.dev/func/pkg/buildpacks"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/s2i"
)

// Scaffolder scaffolds a function for building
type Scaffolder interface {
	Scaffold(ctx context.Context, f fn.Function, path string) error
}

// Builder builds a function image
type Builder interface {
	Build(ctx context.Context, f fn.Function, platforms []fn.Platform) error
}

// TestInt_MiddlewareLabels verifies that the middleware-version label is set
// on function images built by each builder type.
func TestInt_MiddlewareLabels(t *testing.T) {
	tests := []struct {
		name       string
		timeout    time.Duration
		scaffolder Scaffolder
		builder    Builder
	}{
		{
			name:       "s2i",
			timeout:    5 * time.Minute,
			scaffolder: s2i.NewScaffolder(true),
			builder:    s2i.NewBuilder(s2i.WithVerbose(true)),
		},
		{
			name:       "buildpacks",
			timeout:    10 * time.Minute,
			scaffolder: buildpacks.NewScaffolder(true),
			builder:    buildpacks.NewBuilder(buildpacks.WithVerbose(true)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := initFunction(t, "test-"+tt.name+"-labels")
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			if err := tt.scaffolder.Scaffold(ctx, f, ""); err != nil {
				t.Fatal(err)
			}
			if err := tt.builder.Build(ctx, f, nil); err != nil {
				t.Fatal(err)
			}

			assertMiddlewareLabel(t, ctx, f.Build.Image)
		})
	}
}

func initFunction(t *testing.T, name string) fn.Function {
	t.Helper()
	f := fn.Function{
		Name:     name,
		Root:     t.TempDir(),
		Runtime:  "go",
		Registry: "localhost:50000",
	}
	f, err := fn.New().Init(f)
	if err != nil {
		t.Fatal(err)
	}
	f.Build.Image = "localhost:50000/" + name + ":latest"
	return f
}

func assertMiddlewareLabel(t *testing.T, ctx context.Context, image string) {
	t.Helper()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()

	inspect, _, err := cli.ImageInspectWithRaw(ctx, image)
	if err != nil {
		t.Fatalf("failed to inspect image %s: %v", image, err)
	}

	middlewareVersion, ok := inspect.Config.Labels[fn.MiddlewareVersionLabelKey]
	if !ok {
		t.Fatalf("label %q not found in image. Labels: %v", fn.MiddlewareVersionLabelKey, inspect.Config.Labels)
	}
	if middlewareVersion == "" {
		t.Fatalf("label %q is empty", fn.MiddlewareVersionLabelKey)
	}
	t.Logf("middleware-version label: %s", middlewareVersion)
}
