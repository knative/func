//go:build integration
// +build integration

package tekton_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacV1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/knative"

	"knative.dev/func/pkg/builders/buildpacks"
	pack "knative.dev/func/pkg/builders/buildpacks"
	"knative.dev/func/pkg/docker"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/pipelines/tekton"
	"knative.dev/func/pkg/random"

	. "knative.dev/func/pkg/testing"
)

var testCP = func(_ context.Context, _ string) (docker.Credentials, error) {
	return docker.Credentials{
		Username: "",
		Password: "",
	}, nil
}

const (
	TestRegistry = "registry.default.svc.cluster.local:5000"
	// TestRegistry  = "docker.io/alice"
	TestNamespace = "default"
)

func newRemoteTestClient(verbose bool) *fn.Client {
	return fn.New(
		fn.WithBuilder(pack.NewBuilder(pack.WithVerbose(verbose))),
		fn.WithPusher(docker.NewPusher(docker.WithCredentialsProvider(testCP))),
		fn.WithDeployer(knative.NewDeployer(knative.WithDeployerVerbose(verbose))),
		fn.WithRemover(knative.NewRemover(verbose)),
		fn.WithDescriber(knative.NewDescriber(verbose)),
		fn.WithRemover(knative.NewRemover(verbose)),
		fn.WithPipelinesProvider(tekton.NewPipelinesProvider(tekton.WithCredentialsProvider(testCP), tekton.WithVerbose(verbose))),
	)
}

// assertFunctionEchoes returns without error when the function of the given
// name echoes a parameter sent via a Get request.
func assertFunctionEchoes(url string) (err error) {
	token := time.Now().Format("20060102150405.000000000")

	// res, err := http.Get("http://testremote-default.default.127.0.0.1.sslip.io?token=" + token)
	res, err := http.Get(url + "?token=" + token)
	if err != nil {
		return
	}
	if res.StatusCode != 200 {
		return fmt.Errorf("unexpected status code %v", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("error parsing response. %w", err)
	}
	defer res.Body.Close()
	if !strings.Contains(string(body), token) {
		err = fmt.Errorf("response did not contain token. url: %v", url)
		_, _ = httputil.DumpResponse(res, true)
	}
	return
}

func tektonTestsEnabled(t *testing.T) (enabled bool) {
	enabled, _ = strconv.ParseBool(os.Getenv("TEKTON_TESTS_ENABLED"))
	if !enabled {
		t.Log("Tekton tests not enabled.  Enable with TEKTON_TESTS_ENABLED=true")
	}
	return
}

// fromCleanEnvironment of everything except KUBECONFIG. Create a temp directory.
// Change to that temp directory.  Return the curent path as a convenience.
func fromCleanEnvironment(t *testing.T) (root string) {
	// FromTempDirectory clears envs, but sets KUBECONFIG to ./tempdata, so
	// we have to preserve that one value.
	t.Helper()
	kubeconfig := os.Getenv("KUBECONFIG")
	root = FromTempDirectory(t)
	os.Setenv("KUBECONFIG", kubeconfig)
	return
}

func TestRemote_Default(t *testing.T) {
	if !tektonTestsEnabled(t) {
		t.Skip()
	}
	_ = fromCleanEnvironment(t)
	var (
		err         error
		url         string
		verbose     = false
		ctx, cancel = signal.NotifyContext(context.Background(), os.Interrupt)
		client      = newRemoteTestClient(verbose)
	)
	defer cancel()

	f := fn.Function{
		Name:      "testremote-default",
		Runtime:   "node",
		Registry:  TestRegistry,
		Namespace: TestNamespace,
		Build: fn.BuildSpec{
			Builder: "pack", // TODO: test "s2i".  Currently it causes a 'no space left on device' error in GH actions.
		},
	}

	if f, err = client.Init(f); err != nil {
		t.Fatal(err)
	}

	if url, f, err = client.RunPipeline(ctx, f); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = client.Remove(ctx, "", "", f, true)
	}()

	if err := assertFunctionEchoes(url); err != nil {
		t.Fatal(err)
	}
}

func setupNS(t *testing.T) string {
	name := "pipeline-integration-test-" + strings.ToLower(random.AlphaString(5))
	cliSet, err := k8s.NewKubernetesClientset()
	if err != nil {
		t.Fatal(err)
	}
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	_, err = cliSet.CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pp := metav1.DeletePropagationForeground
		_ = cliSet.CoreV1().Namespaces().Delete(context.Background(), name, metav1.DeleteOptions{
			PropagationPolicy: &pp,
		})
	})
	crb := &rbacV1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: name + ":knative-serving-namespaced-admin",
		},
		Subjects: []rbacV1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "default",
				Namespace: name,
			},
		},
		RoleRef: rbacV1.RoleRef{
			Name:     "knative-serving-namespaced-admin",
			Kind:     "ClusterRole",
			APIGroup: "rbac.authorization.k8s.io",
		},
	}
	_, err = cliSet.RbacV1().ClusterRoleBindings().Create(context.Background(), crb, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Log("created namespace: ", name)
	return name
}

func checkTestEnabled(t *testing.T) {
	val := os.Getenv("TEKTON_TESTS_ENABLED")
	enabled, _ := strconv.ParseBool(val)
	if !enabled {
		t.Skip("tekton tests are not enabled")
	}
}

func createSimpleGoProject(t *testing.T, ns string) fn.Function {
	var err error

	funcName := "fn-" + strings.ToLower(random.AlphaString(5))

	projDir := filepath.Join(t.TempDir(), funcName)
	err = os.Mkdir(projDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(projDir, "handle.go"), []byte(simpleGOSvc), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(projDir, "go.mod"), []byte("module function\n\ngo 1.20\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	f := fn.Function{
		Root:     projDir,
		Name:     funcName,
		Runtime:  "none",
		Template: "none",
		Image:    "registry.default.svc.cluster.local:5000/" + funcName,
		Created:  time.Now(),
		Invoke:   "none",
		Build: fn.BuildSpec{
			BuilderImages: map[string]string{
				"pack": buildpacks.DefaultTinyBuilder,
				"s2i":  "registry.access.redhat.com/ubi8/go-toolset",
			},
		},
		Deploy: fn.DeploySpec{
			Namespace: ns,
		},
	}
	f = fn.NewFunctionWith(f)
	err = f.Write()
	if err != nil {
		t.Fatal(err)
	}
	return f
}

const simpleGOSvc = `package function

import (
	"context"
	"net/http"
)

func Handle(ctx context.Context, resp http.ResponseWriter, req *http.Request) {
	resp.Header().Add("Content-Type", "text/plain")
	resp.WriteHeader(200)
	_, _ = resp.Write([]byte("Hello World!\n"))
}
`
