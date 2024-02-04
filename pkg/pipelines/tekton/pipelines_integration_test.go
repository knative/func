//go:build integration
// +build integration

package tekton_test

import (
	"context"
	"net/http"
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

	"knative.dev/func/pkg/builders/buildpacks"
	"knative.dev/func/pkg/docker"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/pipelines/tekton"
	"knative.dev/func/pkg/random"
)

func TestOnClusterBuild(t *testing.T) {
	checkTestEnabled(t)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	credentialsProvider := func(ctx context.Context, image string) (docker.Credentials, error) {
		return docker.Credentials{
			Username: "",
			Password: "",
		}, nil
	}

	tests := []struct {
		Builder string
	}{
		{Builder: "s2i"},
		{Builder: "pack"},
	}

	for _, test := range tests {
		t.Run(test.Builder, func(t *testing.T) {
			if test.Builder == "s2i" {
				t.Skip("Skipping because this causes 'no space left on device' in GH Action.")
			}
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			ns := setupNS(t)

			pp := tekton.NewPipelinesProvider(
				tekton.WithCredentialsProvider(credentialsProvider),
				tekton.WithNamespace(ns))

			f := createSimpleGoProject(t, ns)
			f.Build.Builder = test.Builder

			// simulate deploying by passing the image
			f.Deploy.Image = f.Image

			url, nsReturned, err := pp.Run(ctx, f)
			if err != nil {
				t.Error(err)
				cancel()
			}
			if url == "" {
				t.Error("URL returned is empty")
				cancel()
			}

			if nsReturned == "" || nsReturned != ns {
				t.Errorf("namespace returned is empty or does not match: '%s' should be '%s'", nsReturned, ns)
				cancel()
			}

			resp, err := http.Get(url)
			if err != nil {
				t.Error(err)
				return
			}
			_ = resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Error("bad HTTP response code")
				return
			}
			t.Log("call to knative service successful")
		})
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
