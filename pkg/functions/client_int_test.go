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

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"

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
	// Assemble
	root, cleanup := Mktemp(t)
	defer cleanup()
	verbose := true
	name := "test-new"
	client := newClient(verbose)

	// Act
	if _, _, err := client.New(context.Background(), fn.Function{Name: name, Root: root, Runtime: "go"}); err != nil {
		t.Fatal(err)
	}
	defer del(t, client, name)

	// Assert
	items, err := client.List(context.Background())
	names := []string{}
	for _, item := range items {
		names = append(names, item.Name)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{name}) {
		t.Fatalf("Expected function list ['%v'], got %v", name, names)
	}
}

// TestDeploy deployes using client methods from New but manually
func TestDeploy(t *testing.T) {
	defer Within(t, "testdata/example.com/deploy")()
	verbose := true

	client := newClient(verbose)
	f := fn.Function{Name: "deploy", Root: ".", Runtime: "go"}
	var err error

	if f, err = client.Init(f); err != nil {
		t.Fatal(err)
	}
	if f, err = client.Build(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if f, err = client.Push(context.Background(), f); err != nil {
		t.Fatal(err)
	}

	defer del(t, client, "deploy")
	// TODO: gauron99 -- remove this when you set full image name after build instead
	// of push -- this has to be here because of a workaround
	f.Deploy.Image = f.Build.Image

	if f, err = client.Deploy(context.Background(), f); err != nil {
		t.Fatal(err)
	}
}

// TestDeployWithOptions deploys function with all options explicitly set
func TestDeployWithOptions(t *testing.T) {
	root, cleanup := Mktemp(t)
	defer cleanup()
	verbose := false

	f := fn.Function{Runtime: "go", Name: "test-deploy-with-options", Root: root, Namespace: DefaultNamespace}
	f.Deploy = fn.DeploySpec{
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
	if _, _, err := client.New(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	defer del(t, client, "test-deploy-with-options")
}

func TestDeployWithTriggers(t *testing.T) {
	root, cleanup := Mktemp(t)
	defer cleanup()
	verbose := true

	f := fn.Function{Runtime: "go", Name: "test-deploy-with-triggers", Root: root}
	f.Deploy = fn.DeploySpec{
		Subscriptions: []fn.KnativeSubscription{
			{
				Source: "default",
				Filters: map[string]string{
					"key": "value",
					"foo": "bar",
				},
			},
		},
	}

	client := newClient(verbose)
	if _, _, err := client.New(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	defer del(t, client, "test-deploy-with-triggers")
}

func TestUpdateWithAnnotationsAndLabels(t *testing.T) {
	functionName := "updateannlab"
	defer Within(t, "testdata/example.com/"+functionName)()
	verbose := false

	servingClient, err := knative.NewServingClient(DefaultNamespace)

	// Deploy a function without any annotations or labels
	client := newClient(verbose)
	f := fn.Function{Name: functionName, Root: ".", Runtime: "go"}

	if _, f, err = client.New(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	defer del(t, client, functionName)

	// Updated function with a new set of annotations and labels
	// deploy and check that deployed kcsv contains correct annotations and labels

	annotations := map[string]string{"ann1": "val1", "ann2": "val2"}
	labels := []fn.Label{
		{Key: ptr.String("lab1"), Value: ptr.String("v1")},
		{Key: ptr.String("lab2"), Value: ptr.String("v2")},
	}
	f.Deploy.Annotations = annotations
	f.Deploy.Labels = labels

	if f, err = client.Deploy(context.Background(), f, fn.WithDeploySkipBuildCheck(true)); err != nil {
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

	annotations = map[string]string{"ann1": "val1"}
	labels = []fn.Label{{Key: ptr.String("lab1"), Value: ptr.String("v1")}}
	f.Deploy.Annotations = annotations
	f.Deploy.Labels = labels

	if f, err = client.Deploy(context.Background(), f, fn.WithDeploySkipBuildCheck(true)); err != nil {
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
	f := fn.Function{Name: "remove", Root: ".", Runtime: "go"}
	var err error
	if _, f, err = client.New(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	waitFor(t, client, "remove")

	if err = client.Remove(context.Background(), f, false); err != nil {
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
	_, err := client.Init(fn.Function{
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
	f, err = client.Init(f)
	if err != nil {
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

	if route, f, err = client.Apply(ctx, f); err != nil {
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
	f := fn.Function{Name: "a", Runtime: "go"}
	f, err = client.Init(f)
	if err != nil {
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
	if _, f, err = client.Apply(ctx, f); err != nil {
		t.Fatal(err)
	}
	defer client.Remove(ctx, f, true)

	// Create Function B
	// which responds with the response from an invocation of 'a' via the
	// localhost service discovery and invocation API.
	root, done = Mktemp(t)
	defer done()
	f = fn.Function{Name: "b", Runtime: "go"}
	f, err = client.Init(f)
	if err != nil {
		t.Fatal(err)
	}

	source = `
package function

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func Handle(ctx context.Context, w http.ResponseWriter, req *http.Request) {
	var e error
	var r *http.Response
	var buff bytes.Buffer
	var out io.Writer = io.MultiWriter(os.Stderr, &buff)
	for i := 0; i < 10; i++ {
		r, e = http.Get("http://localhost:3500/v1.0/invoke/a/method/")
		if e != nil {
			_, _ = fmt.Fprintf(out, "unable to invoke function a: %v\n", e)
			time.Sleep(time.Second*3)
			continue
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			_, _ = fmt.Fprintf(out, "bad http status code when invoking a: %d\n", r.StatusCode)
			time.Sleep(time.Second*3)
			continue
		}
		w.WriteHeader(200)
		_, _ = io.Copy(w, r.Body)
		return
	}
	http.Error(w, fmt.Sprintf("unable to invoke function a:\n%s", buff.String()), http.StatusServiceUnavailable)
	return
}
`
	os.WriteFile(filepath.Join(root, "handle.go"), []byte(source), os.ModePerm)
	if route, f, err = client.Apply(ctx, f); err != nil {
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
		t.Fatalf("Unexpected response from Function B: %v", string(body))
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
	remover := knative.NewRemover(verbose)
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
	if err := c.Remove(context.Background(), fn.Function{Name: name, Deploy: fn.DeploySpec{Namespace: DefaultNamespace}}, false); err != nil {
		t.Fatal(err)
	}
	cli, _, err := docker.NewClient(client.DefaultDockerHost)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	opts := volume.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("name", fmt.Sprintf("pack-cache-func_%s_*", name)),
		),
	}
	resp, err := cli.VolumeList(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, vol := range resp.Volumes {
		t.Log("deleting volume:", vol.Name)
		err = cli.VolumeRemove(context.Background(), vol.Name, true)
		if err != nil {
			t.Fatal(err)
		}
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
