//go:build integration
// +build integration

package functions_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"knative.dev/func/pkg/builders/buildpacks"
	"knative.dev/func/pkg/docker"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/knative"
	. "knative.dev/func/pkg/testing"
	"knative.dev/pkg/ptr"
)

// # Integration Tests
//
// go test -tags integration ./...
//
// ## Cluster Required
//
// These integration tests require a properly configured cluster,
// such as that which is setup and configured in CI (see .github/workflows).
// Linux developers can set up the cluster via:
//
//   ./hack/binaries.sh && ./hack/allocate.sh && ./hack/registry.sh
//
// ## Cluster Cleanup
//
// The test cluster and most resources can be removed with:
//   ./hack/delete.sh
//
// NOTE: Downloaded images are not removed.
//

const (
	// DefaultRegistry must contain both the registry host and
	// registry namespace at this time.  This will likely be
	// split and defaulted to the forthcoming in-cluster registry.
	DefaultRegistry = "localhost:50000/func"

	// DefaultNamespace for the underlying deployments.  Must be the same
	// as is set up and configured (see hack/configure.sh)
	DefaultNamespace = "func"
)

func TestList(t *testing.T) {
	verbose := true

	// Assemble
	lister := knative.NewLister(DefaultNamespace, verbose)

	client := fn.New(
		fn.WithLister(lister),
		fn.WithVerbose(verbose))

	// Act
	names, err := client.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if len(names) != 0 {
		t.Fatalf("Expected no functions, got %v", names)
	}
}

// TestNew creates
func TestNew(t *testing.T) {
	defer Within(t, "testdata/example.com/testnew")()
	verbose := true

	client := newClient(verbose)

	// Act
	if _, err := client.New(context.Background(), fn.Function{Name: "testnew", Root: ".", Runtime: "go"}); err != nil {
		t.Fatal(err)
	}
	defer del(t, client, "testnew")

	// Assert
	items, err := client.List(context.Background())
	names := []string{}
	for _, item := range items {
		names = append(names, item.Name)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{"testnew"}) {
		t.Fatalf("Expected function list ['testnew'], got %v", names)
	}
}

// TestDeploy updates
func TestDeploy(t *testing.T) {
	defer Within(t, "testdata/example.com/deploy")()
	verbose := true

	client := newClient(verbose)

	if _, err := client.New(context.Background(), fn.Function{Name: "deploy", Root: ".", Runtime: "go"}); err != nil {
		t.Fatal(err)
	}
	defer del(t, client, "deploy")

	if err := client.Deploy(context.Background(), "."); err != nil {
		t.Fatal(err)
	}
}

// TestDeployWithOptions deploys function with all options explicitly set
func TestDeployWithOptions(t *testing.T) {
	defer Within(t, "testdata/example.com/deployoptions")()
	verbose := true

	ds := fn.DeploySpec{
		Options: fn.Options{
			Scale: &fn.ScaleOptions{
				Min:         ptr.Int64(1),
				Max:         ptr.Int64(10),
				Metric:      ptr.String("concurrency"),
				Target:      ptr.Float64(5),
				Utilization: ptr.Float64(5),
			},
			Resources: &fn.ResourcesOptions{
				Requests: &fn.ResourcesRequestsOptions{
					CPU:    ptr.String("10m"),
					Memory: ptr.String("100m"),
				},
				Limits: &fn.ResourcesLimitsOptions{
					CPU:         ptr.String("1000m"),
					Memory:      ptr.String("1000M"),
					Concurrency: ptr.Int64(10),
				},
			},
		},
	}

	client := newClient(verbose)

	if _, err := client.New(context.Background(), fn.Function{Name: "deployoptions", Root: ".", Runtime: "go", Deploy: ds}); err != nil {
		t.Fatal(err)
	}
	defer del(t, client, "deployoptions")

	if err := client.Deploy(context.Background(), "."); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateWithAnnotationsAndLabels(t *testing.T) {
	functionName := "updateannlab"
	defer Within(t, "testdata/example.com/"+functionName)()
	verbose := true

	servingClient, err := knative.NewServingClient(DefaultNamespace)

	// Deploy a function without any annotations or labels
	client := newClient(verbose)

	if _, err := client.New(context.Background(), fn.Function{Name: functionName, Root: ".", Runtime: "go"}); err != nil {
		t.Fatal(err)
	}
	defer del(t, client, functionName)

	if err := client.Deploy(context.Background(), "."); err != nil {
		t.Fatal(err)
	}

	// Update function with a new set of annotations and labels
	// deploy and check that deployed kcsv contains correct annotations and labels
	f, err := fn.NewFunction(".")
	if err != nil {
		t.Fatal(err)
	}

	annotations := map[string]string{"ann1": "val1", "ann2": "val2"}
	labels := []fn.Label{
		{Key: ptr.String("lab1"), Value: ptr.String("v1")},
		{Key: ptr.String("lab2"), Value: ptr.String("v2")},
	}
	f.Deploy.Annotations = annotations
	f.Deploy.Labels = labels
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}

	if err := client.Deploy(context.Background(), ".", fn.WithDeploySkipBuildCheck(true)); err != nil {
		t.Fatal(err)
	}

	ksvc, err := servingClient.GetService(context.Background(), functionName)
	if err != nil {
		t.Fatal(err)
	}

	for k, v := range annotations {
		if val, ok := ksvc.Annotations[k]; ok {
			if v != val {
				t.Fatal(fmt.Errorf("expected annotation %q to have value %q, but the annotation in the deployed service has value %q", k, v, val))
			}
		} else {
			t.Fatal(fmt.Errorf("annotation %q not found in the deployed service", k))
		}
	}
	for _, l := range labels {
		if val, ok := ksvc.Labels[*l.Key]; ok {
			if *l.Value != val {
				t.Fatal(fmt.Errorf("expected label %q to have value %q, but the label in the deployed service has value %q", *l.Key, *l.Value, val))
			}
		} else {
			t.Fatal(fmt.Errorf("label %q not found in the deployed service", *l.Key))
		}
	}

	// Remove some annotations and labels
	// deploy and check that deployed kcsv contains correct annotations and labels
	f, err = fn.NewFunction(".")
	if err != nil {
		t.Fatal(err)
	}

	annotations = map[string]string{"ann1": "val1"}
	labels = []fn.Label{{Key: ptr.String("lab1"), Value: ptr.String("v1")}}
	f.Deploy.Annotations = annotations
	f.Deploy.Labels = labels
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}

	if err := client.Deploy(context.Background(), ".", fn.WithDeploySkipBuildCheck(true)); err != nil {
		t.Fatal(err)
	}

	ksvc, err = servingClient.GetService(context.Background(), functionName)
	if err != nil {
		t.Fatal(err)
	}

	for k, v := range annotations {
		if val, ok := ksvc.Annotations[k]; ok {
			if v != val {
				t.Fatal(fmt.Errorf("expected annotation %q to have value %q, but the annotation in the deployed service has value %q", k, v, val))
			}
		} else {
			t.Fatal(fmt.Errorf("annotation %q not found in the deployed service", k))
		}
	}
	for _, l := range labels {
		if val, ok := ksvc.Labels[*l.Key]; ok {
			if *l.Value != val {
				t.Fatal(fmt.Errorf("expected label %q to have value %q, but the label in the deployed service has value %q", *l.Key, *l.Value, val))
			}
		} else {
			t.Fatal(fmt.Errorf("label %q not found in the deployed service", *l.Key))
		}
	}
}

// TestRemove deletes
func TestRemove(t *testing.T) {
	defer Within(t, "testdata/example.com/remove")()
	verbose := true

	client := newClient(verbose)

	if _, err := client.New(context.Background(), fn.Function{Name: "remove", Root: ".", Runtime: "go"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, client, "remove")

	if err := client.Remove(context.Background(), fn.Function{Name: "remove"}, false); err != nil {
		t.Fatal(err)
	}

	names, err := client.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Fatalf("Expected empty functions list, got %v", names)
	}
}

// TestRemoteRepositories ensures that initializing a function
// defined in a remote repository finds the template, writes
// the expected files, and retains the expected modes.
// NOTE: this test only succeeds due to an override in
// templates' copyNode which forces mode 755 for directories.
// See https://github.com/go-git/go-git/issues/364
func TestRemoteRepositories(t *testing.T) {
	defer Within(t, "testdata/example.com/remote")()

	// Write the test template from the remote onto root
	client := fn.New(
		fn.WithRegistry(DefaultRegistry),
		fn.WithRepository("https://github.com/boson-project/test-templates"),
	)
	err := client.Init(fn.Function{
		Root:     ".",
		Runtime:  "runtime",
		Template: "template",
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		Path string
		Perm uint32
		Dir  bool
	}{
		{Path: "file", Perm: 0644},
		{Path: "dir-a/file", Perm: 0644},
		{Path: "dir-b/file", Perm: 0644},
		{Path: "dir-b/executable", Perm: 0755},
		{Path: "dir-b", Perm: 0755},
		{Path: "dir-a", Perm: 0755},
	}

	// Note that .Perm() are used to only consider the least-signifigant 9 and
	// thus not have to consider the directory bit.
	for _, test := range tests {
		file, err := os.Stat(test.Path)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%04o repository/%v", file.Mode().Perm(), test.Path)
		if file.Mode().Perm() != os.FileMode(test.Perm) {
			t.Fatalf("expected 'repository/%v' to have mode %04o, got %04o", test.Path, test.Perm, file.Mode().Perm())
		}
	}
}

// TestInvoke_ClientToService ensures that the client can invoke a remotely
// deployed service, both by the route returned directly as well as using
// the invocation helper client.Invoke.
func TestInvoke_ClientToService(t *testing.T) {
	var (
		root, done = Mktemp(t)
		verbose    = true
		ctx        = context.Background()
		client     = newClient(verbose)
		route      string
		err        error
	)
	defer done()

	// Create a function
	f := fn.Function{Name: "f", Runtime: "go"}
	if err = client.Init(f); err != nil {
		t.Fatal(err)
	}
	source := `
package function

import (
  "context"
  "net/http"
)

func Handle(ctx context.Context, res http.ResponseWriter, req *http.Request) {
  res.Write([]byte("TestInvoke_ClientToService OK"))
}
`
	os.WriteFile(filepath.Join(root, "handle.go"), []byte(source), os.ModePerm)

	if route, err = client.Apply(ctx, f); err != nil {
		t.Fatal(err)
	}
	defer client.Remove(ctx, f, true)

	// Invoke via the route
	resp, err := http.Get(route)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if string(b) != "TestInvoke_ClientToService OK" {
		t.Fatalf("unexpected response from HTTP GET: %v", b)
	}

	// Invoke using the helper
	_, body, err := client.Invoke(ctx, root, "", fn.NewInvokeMessage())
	if err != nil {
		t.Fatal(err)
	}

	if body != "TestInvoke_ClientToService OK" {
		t.Fatalf("unexpected response from client.Invoke: %v", b)
	}
}

// TestInvoke_ServiceToService ensures that a Function can invoke another
// service via localhost service discovery api provided by the Dapr sidecar.
func TestInvoke_ServiceToService(t *testing.T) {
	var (
		verbose = true
		ctx     = context.Background()
		client  = newClient(verbose)
		err     error
		source  string
		route   string
	)

	// Create function A
	// A function which responds to GET requests with a static value.
	root, done := Mktemp(t)
	defer done()
	f := fn.Function{Name: "a", Runtime: "go", Enable: []string{"dapr"}}
	if err := client.Init(f); err != nil {
		t.Fatal(err)
	}
	source = `
package function

import (
  "context"
  "net/http"
)

func Handle(ctx context.Context, res http.ResponseWriter, req *http.Request) {
  res.Write([]byte("TestInvoke_ServiceToService OK"))
}
`
	os.WriteFile(filepath.Join(root, "handle.go"), []byte(source), os.ModePerm)
	if _, err = client.Apply(ctx, f); err != nil {
		t.Fatal(err)
	}
	defer client.Remove(ctx, f, true)

	// Create Function B
	// which responds with the response from an invocation of 'a' via the
	// localhost service discovery and invocation API.
	root, done = Mktemp(t)
	defer done()
	f = fn.Function{Name: "b", Runtime: "go", Enable: []string{"dapr"}}
	if err := client.Init(f); err != nil {
		t.Fatal(err)
	}

	source = `
package function

import (
  "context"
  "net/http"
	"fmt"
	"io"
)

func Handle(ctx context.Context, res http.ResponseWriter, req *http.Request) {
	r, err := http.Get("http://localhost:3500/v1.0/invoke/a/method/")
	if err != nil {
		fmt.Printf("unable to invoke function a: %v", err)
	  http.Error(res, "unable to invoke function a", http.StatusServiceUnavailable)
	}
	defer r.Body.Close()
	io.Copy(res,r.Body)
}
`
	os.WriteFile(filepath.Join(root, "handle.go"), []byte(source), os.ModePerm)
	if route, err = client.Apply(ctx, f); err != nil {
		t.Fatal(err)
	}
	defer client.Remove(ctx, f, true)

	resp, err := http.Get(route)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	fmt.Printf("### function a response body: %s\n", body)

	if string(body) != "TestInvoke_ServiceToService OK" {
		t.Fatalf("Unexpected response from Function B: %s", body)
	}
}

// ***********
//   Helpers
// ***********

// newClient creates an instance of the func client with concrete impls
// sufficient for running integration tests.
func newClient(verbose bool) *fn.Client {
	builder := buildpacks.NewBuilder(buildpacks.WithVerbose(verbose))
	pusher := docker.NewPusher(docker.WithVerbose(verbose))
	deployer := knative.NewDeployer(knative.WithDeployerNamespace(DefaultNamespace), knative.WithDeployerVerbose(verbose))
	describer := knative.NewDescriber(DefaultNamespace, verbose)
	remover := knative.NewRemover(DefaultNamespace, verbose)
	lister := knative.NewLister(DefaultNamespace, verbose)

	return fn.New(
		fn.WithRegistry(DefaultRegistry),
		fn.WithVerbose(verbose),
		fn.WithBuilder(builder),
		fn.WithPusher(pusher),
		fn.WithDeployer(deployer),
		fn.WithDescriber(describer),
		fn.WithRemover(remover),
		fn.WithLister(lister),
	)
}

// Del cleans up after a test by removing a function by name.
// (test fails if the named function does not exist)
//
// Intended to be run in a defer statement immediately after creation, del
// works around the asynchronicity of the underlying platform's creation
// step by polling the provider until the names function becomes available
// (or the test times out), before firing off a deletion request.
// Of course, ideally this would be replaced by the use of a synchronous
// method, or at a minimum a way to register a callback/listener for the
// creation event.  This is what we have for now, and the show must go on.
func del(t *testing.T, c *fn.Client, name string) {
	t.Helper()
	waitFor(t, c, name)
	if err := c.Remove(context.Background(), fn.Function{Name: name}, false); err != nil {
		t.Fatal(err)
	}
}

// waitFor the named function to become available in List output.
// TODO: the API should be synchronous, but that depends first on
// Create returning the derived name such that we can bake polling in.
// Ideally the provider's Create would be made syncrhonous.
func waitFor(t *testing.T, c *fn.Client, name string) {
	t.Helper()
	var pollInterval = 2 * time.Second

	for { // ever (i.e. defer to global test timeout)
		nn, err := c.List(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range nn {
			if n.Name == name {
				return
			}
		}
		time.Sleep(pollInterval)
	}
}
