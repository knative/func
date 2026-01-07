//go:build integration
// +build integration

package functions_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"knative.dev/func/pkg/docker"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/knative"
	"knative.dev/func/pkg/oci"
	"knative.dev/func/pkg/s2i"
	. "knative.dev/func/pkg/testing"
	"knative.dev/pkg/ptr"
)

// # Integration Tests
//
// go test -tags integration ./...
//
// ## Requirements
//
// A cluster is required. See .github/workflows for more. For example:
//
//   ./hack/binaries.sh && ./hack/cluster.sh && ./hack/registry.sh
//
// Binaries are required:  go for compiling functions and git for
// repository-related tests.
//
// ## Cluster Cleanup
//
// The test cluster and most resources can be removed with:
//   ./hack/delete.sh
//
// ## Configuration
//
// Use the FUNC_INT_* environment variables to alter behavior, binaries to
// use, etc.
//
// NOTE: Downloaded images are not removed.
//

const (
	DefaultIntTestHome       = "./testdata/default_home"
	DefaultIntTestKubeconfig = "../../hack/bin/kubeconfig.yaml"
	DefaultIntTestNamespace  = "default"
	DefaultIntTestVerbose    = false
	// DefaultIntTestRegistry = // see testing package (it's shared)
)

var (
	Go         = getEnvAsBin("FUNC_INT_GO", "go")
	GitBin     = getEnvAsBin("FUNC_INT_GIT", "git")
	Kubeconfig = getEnvAsPath("FUNC_INT_KUBECONFIG", DefaultIntTestKubeconfig)
	Verbose    = getEnvAsBool("FUNC_INT_VERBOSE", DefaultIntTestVerbose)
	Home, _    = filepath.Abs(DefaultIntTestHome)
	//Registry = // see testing package (it's shared)
)

// containsInstance checks if the list includes the given instance.
func containsInstance(list []fn.ListItem, name, namespace string) bool {
	for _, v := range list {
		if v.Name == name && v.Namespace == namespace {
			return true
		}
	}
	return false
	// Note that client.List is tested implicitly via its use in TestInt_New
	// and TestInt_Delete.
}

// TestInt_New creates
func TestInt_New(t *testing.T) {
	resetEnv()
	// Assemble
	root, cleanup := Mktemp(t)
	defer cleanup()
	verbose := true
	name := "test-new"
	client := newClient(verbose)

	// Act
	if _, _, err := client.New(context.Background(), fn.Function{Name: name, Namespace: DefaultIntTestNamespace, Root: root, Runtime: "go"}); err != nil {
		t.Fatal(err)
	}
	defer del(t, client, name, DefaultIntTestNamespace)

	// Assert
	list, err := client.List(context.Background(), DefaultIntTestNamespace)
	if err != nil {
		t.Fatal(err)
	}

	if !containsInstance(list, name, DefaultIntTestNamespace) {
		t.Log(list)
		t.Fatalf("deployed instance list does not contain function %q", name)
	}
}

// TestInt_Deploy_Defaults deploys using client methods from New but manually
func TestInt_Deploy_Defaults(t *testing.T) {
	resetEnv()
	_, cleanup := Mktemp(t)
	defer cleanup()
	verbose := true

	client := newClient(verbose)
	f := fn.Function{Name: "deploy", Namespace: DefaultIntTestNamespace, Runtime: "go"}
	var err error

	if f, err = client.Init(f); err != nil {
		t.Fatal(err)
	}
	if f, err = client.Build(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if f, _, err = client.Push(context.Background(), f); err != nil {
		t.Fatal(err)
	}

	defer del(t, client, "deploy", DefaultIntTestNamespace)
	// TODO: gauron99 -- remove this when you set full image name after build instead
	// of push -- this has to be here because of a workaround
	f.Deploy.Image = f.Build.Image

	if f, err = client.Deploy(context.Background(), f); err != nil {
		t.Fatal(err)
	}
}

// TestInt_Deploy_WithOptions deploys function with all options explicitly set
func TestInt_Deploy_WithOptions(t *testing.T) {
	resetEnv()
	root, cleanup := Mktemp(t)
	defer cleanup()
	verbose := false

	f := fn.Function{Runtime: "go", Name: "test-deploy-with-options", Root: root, Namespace: DefaultIntTestNamespace}
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
	defer del(t, client, "test-deploy-with-options", DefaultIntTestNamespace)
}

func TestInt_Deploy_WithTriggers(t *testing.T) {
	resetEnv()
	root, cleanup := Mktemp(t)
	defer cleanup()
	verbose := true

	f := fn.Function{Runtime: "go", Name: "test-deploy-with-triggers", Root: root, Namespace: DefaultIntTestNamespace}
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
	defer del(t, client, "test-deploy-with-triggers", DefaultIntTestNamespace)
}

func TestInt_Update_WithAnnotationsAndLabels(t *testing.T) {
	resetEnv()
	_, cleanup := Mktemp(t)
	defer cleanup()
	functionName := "updateannlab"
	verbose := false

	servingClient, err := knative.NewServingClient(DefaultIntTestNamespace)
	if err != nil {
		t.Fatal(err)
	}

	// Deploy a function without any annotations or labels
	client := newClient(verbose)
	f := fn.Function{Name: functionName, Runtime: "go", Namespace: DefaultIntTestNamespace}

	if _, f, err = client.New(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	defer del(t, client, functionName, DefaultIntTestNamespace)

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

// TestInt_Remove ensures removal of a function instance.
func TestInt_Remove(t *testing.T) {
	resetEnv()
	_, cleanup := Mktemp(t)
	defer cleanup()
	verbose := true
	name := "remove"

	client := newClient(verbose)
	f := fn.Function{Name: name, Namespace: DefaultIntTestNamespace, Runtime: "go"}
	var err error
	if _, _, err = client.New(context.Background(), f); err != nil {
		t.Fatal(err)
	}

	del(t, client, "remove", DefaultIntTestNamespace)

	list, err := client.List(context.Background(), DefaultIntTestNamespace)
	if err != nil {
		t.Fatal(err)
	}
	if containsInstance(list, name, DefaultIntTestNamespace) {
		t.Log(list)
		t.Fatalf("deployed instance list still contains function %q", name)
	}
}

// TestInt_RemoteRepositories ensures that initializing a function
// defined in a remote repository finds the template, writes
// the expected files, and retains the expected modes.
// NOTE: this test only succeeds due to an override in
// templates' copyNode which forces mode 755 for directories.
// See https://github.com/go-git/go-git/issues/364
func TestInt_RemoteRepositories(t *testing.T) {
	resetEnv()
	_, cleanup := Mktemp(t)
	defer cleanup()

	// Write the test template from the remote onto root
	client := fn.New(
		fn.WithRegistry(DefaultIntTestRegistry),
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

	// Note that .Perm() are used to only consider the least-significant 9 and
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

// TestInt_Invoke_ClientToService ensures that the client can invoke a remotely
// deployed service, both by the route returned directly as well as using
// the invocation helper client.Invoke.
func TestInt_Invoke_ClientToService(t *testing.T) {
	resetEnv()
	root, cleanup := Mktemp(t)
	defer cleanup()
	var (
		verbose = true
		ctx     = context.Background()
		client  = newClient(verbose)
		route   string
		err     error
	)

	// Create a function
	f := fn.Function{Name: "f", Runtime: "go", Namespace: DefaultIntTestNamespace}
	f, err = client.Init(f)
	if err != nil {
		t.Fatal(err)
	}
	source := `
package function

import "net/http"

func Handle(res http.ResponseWriter, req *http.Request) {
  res.Write([]byte("TestInvoke_ClientToService OK"))
}
`
	err = os.WriteFile(filepath.Join(root, "handle.go"), []byte(source), os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}

	if route, f, err = client.Apply(ctx, f); err != nil {
		t.Fatal(err)
	}
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}
	defer del(t, client, "f", DefaultIntTestNamespace)

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

// TestInt_Invoke_ServiceToService ensures that a Function can invoke another
// service via localhost service discovery api provided by the Dapr sidecar.
func TestInt_Invoke_ServiceToService(t *testing.T) {
	resetEnv()
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
	f := fn.Function{Name: "a", Runtime: "go", Namespace: DefaultIntTestNamespace}
	f, err = client.Init(f)
	if err != nil {
		t.Fatal(err)
	}
	source = `
package function

import "net/http"

func Handle(res http.ResponseWriter, req *http.Request) {
  res.Write([]byte("TestInvoke_ServiceToService OK"))
}
`
	err = os.WriteFile(filepath.Join(root, "handle.go"), []byte(source), os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = client.Apply(ctx, f); err != nil {
		t.Fatal(err)
	}
	defer del(t, client, "a", DefaultIntTestNamespace)

	// Create Function B
	// which responds with the response from an invocation of 'a' via the
	// localhost service discovery and invocation API.
	client2 := newClient(verbose)
	root, done = Mktemp(t)
	defer done()
	f = fn.Function{Name: "b", Runtime: "go", Namespace: DefaultIntTestNamespace}
	f, err = client2.Init(f)
	if err != nil {
		t.Fatal(err)
	}

	source = `
package function

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func Handle(w http.ResponseWriter, req *http.Request) {
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
	err = os.WriteFile(filepath.Join(root, "handle.go"), []byte(source), os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}
	if route, f, err = client2.Apply(ctx, f); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client2.Remove(ctx, "", "", f, true) }()

	resp, err := http.Get(route)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if string(body) != "TestInvoke_ServiceToService OK" {
		t.Fatalf("Unexpected response from Function B: %v", string(body))
	}
}

// TestDeployWithoutHome ensures that running client.New works without
// home
func TestInt_DeployWithoutHome(t *testing.T) {
	root, cleanup := Mktemp(t)
	defer cleanup()

	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	verbose := false
	name := "test-deploy-no-home"

	f := fn.Function{Runtime: "go", Name: name, Root: root, Namespace: DefaultIntTestNamespace}

	// client with s2i builder because pack needs HOME
	client := newClientWithS2i(verbose)

	// expect to succeed
	_, _, err := client.New(context.Background(), f)
	if err != nil {
		t.Fatalf("expected no errors but got %v", err)
	}

	defer del(t, client, name, DefaultIntTestNamespace)
}

// ***********
//	Helpers
// ***********

func getEnvAsPath(env, dflt string) (val string) {
	val = getEnv(env, dflt)
	if !filepath.IsAbs(val) { // convert to abs
		var err error
		if val, err = filepath.Abs(val); err != nil {
			panic(fmt.Sprintf("error converting path to absolute. %v", err))
		}
	}
	return
}

func getEnvAsBool(env string, dfltBool bool) bool {
	dflt := fmt.Sprintf("%t", dfltBool)
	val, err := strconv.ParseBool(getEnv(env, dflt))
	if err != nil {
		panic(fmt.Sprintf("value for %v expected to be boolean. %v", env, err))
	}
	return val
}

func getEnvAsBin(env, dflt string) string {
	val, err := exec.LookPath(getEnv(env, dflt))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error locating command %q. %v", val, err)
	}
	return val
}

func getEnv(env, dflt string) (val string) {
	if v := os.Getenv(env); v != "" {
		val = v
	}
	if val == "" {
		val = dflt
	}
	return
}

func resetEnv() {
	os.Clearenv()
	os.Setenv("HOME", Home)
	os.Setenv("KUBECONFIG", Kubeconfig)
	os.Setenv("FUNC_GO", Go)
	os.Setenv("FUNC_GIT", GitBin)
	os.Setenv("FUNC_VERBOSE", fmt.Sprintf("%t", Verbose))

	// The Registry will be set either during first-time setup using the
	// global config, or already defaulted by the user via environment variable.
	os.Setenv("FUNC_REGISTRY", Registry())

	// The following host-builder related settings will become the defaults
	// once the host builder supports the core runtimes.  Setting them here in
	// order to futureproof individual tests.
	os.Setenv("FUNC_BUILDER", "host")    // default to host builder
	os.Setenv("FUNC_CONTAINER", "false") // "run" uses host builder

}

// newClient creates an instance of the func client with concrete impls
// sufficient for running integration tests.
func newClient(verbose bool) *fn.Client {
	return fn.New(
		fn.WithRegistry(DefaultIntTestRegistry),
		fn.WithBuilder(oci.NewBuilder("", verbose)),
		fn.WithPusher(oci.NewPusher(true, true, verbose)),
		fn.WithDeployer(knative.NewDeployer(knative.WithDeployerVerbose(verbose))),
		fn.WithDescribers(knative.NewDescriber(verbose), k8s.NewDescriber(verbose)),
		fn.WithRemovers(knative.NewRemover(verbose), k8s.NewRemover(verbose)),
		fn.WithListers(knative.NewLister(verbose), k8s.NewLister(verbose)),
		fn.WithVerbose(verbose),
	)
}

// copy of newClient just builder is s2i instead of buildpacks
func newClientWithS2i(verbose bool) *fn.Client {
	builder := s2i.NewBuilder(s2i.WithVerbose(verbose))
	pusher := docker.NewPusher(docker.WithVerbose(verbose))
	deployer := knative.NewDeployer(knative.WithDeployerVerbose(verbose))

	return fn.New(
		fn.WithRegistry(DefaultIntTestRegistry),
		fn.WithVerbose(verbose),
		fn.WithBuilder(builder),
		fn.WithPusher(pusher),
		fn.WithDeployer(deployer),
		fn.WithDescribers(knative.NewDescriber(verbose), k8s.NewDescriber(verbose)),
		fn.WithRemovers(knative.NewRemover(verbose), k8s.NewRemover(verbose)),
		fn.WithListers(knative.NewLister(verbose), k8s.NewLister(verbose)),
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
func del(t *testing.T, c *fn.Client, name, namespace string) {
	t.Helper()
	waitFor(t, c, name, namespace)
	f := fn.Function{Name: name, Deploy: fn.DeploySpec{Namespace: DefaultIntTestNamespace}}
	if err := c.Remove(context.Background(), "", "", f, false); err != nil {
		t.Fatal(err)
	}

	// TODO(lkingland):  The below breaks things
	// The following should not exist, nor its imports.
	//
	// If there is ever a problem with dangling volumes, that is
	// something which should be fixed in the _remover_, and tested there.
	//
	// ... check with Jefferson what caused such a state, open an
	// issue to alter remover if necessary, and then delete all of this.
	if true {
		return
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
func waitFor(t *testing.T, c *fn.Client, name, namespace string) {
	t.Helper()
	var pollInterval = 2 * time.Second

	for { // ever (i.e. defer to global test timeout)
		nn, err := c.List(context.Background(), namespace)
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
