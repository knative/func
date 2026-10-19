package operator

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakediscovery "k8s.io/client-go/discovery/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/functions-dev/func-operator/api/v1alpha1"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	return s
}

func fakeDiscoveryWithCRD() *fakediscovery.FakeDiscovery {
	cs := fakeclientset.NewSimpleClientset()
	fd := cs.Discovery().(*fakediscovery.FakeDiscovery)
	fd.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: v1alpha1.GroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "functions", Kind: "Function"},
			},
		},
	}
	return fd
}

func fakeDiscoveryWithoutCRD() *fakediscovery.FakeDiscovery {
	cs := fakeclientset.NewSimpleClientset()
	return cs.Discovery().(*fakediscovery.FakeDiscovery)
}

func TestSyncFunctionCR_CreateNew(t *testing.T) {
	scheme := newScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	disc := fakeDiscoveryWithCRD()

	cfg := SyncConfig{
		FunctionName: "my-func",
		Namespace:    "default",
		RepoURL:      "https://github.com/alice/my-func.git",
		RepoRevision: "main",
		RepoDir:      ".",
	}

	err := syncFunctionCR(context.Background(), cl, disc, cfg)
	if err != nil {
		t.Fatal(err)
	}

	var fn v1alpha1.Function
	err = cl.Get(context.Background(), ctrlclient.ObjectKey{
		Name:      "my-func",
		Namespace: "default",
	}, &fn)
	if err != nil {
		t.Fatalf("expected Function CR to be created: %v", err)
	}
	if fn.Spec.Repository.URL != "https://github.com/alice/my-func.git" {
		t.Fatalf("expected repo URL, got %q", fn.Spec.Repository.URL)
	}
	if fn.Spec.Repository.Revision != "main" {
		t.Fatalf("expected revision 'main', got %q", fn.Spec.Repository.Revision)
	}
	if fn.Spec.Repository.Dir != "." {
		t.Fatalf("expected dir '.', got %q", fn.Spec.Repository.Dir)
	}
}

func TestSyncFunctionCR_UpdateExistingByMetadataName(t *testing.T) {
	scheme := newScheme()
	existing := &v1alpha1.Function{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-func",
			Namespace: "default",
		},
		Spec: v1alpha1.FunctionSpec{
			Repository: v1alpha1.FunctionSpecRepository{
				URL:      "https://github.com/old/repo.git",
				Revision: "old-branch",
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	disc := fakeDiscoveryWithCRD()

	cfg := SyncConfig{
		FunctionName: "my-func",
		Namespace:    "default",
		RepoURL:      "https://github.com/alice/my-func.git",
		RepoRevision: "main",
		RepoDir:      "subfolder",
	}

	err := syncFunctionCR(context.Background(), cl, disc, cfg)
	if err != nil {
		t.Fatal(err)
	}

	var fn v1alpha1.Function
	err = cl.Get(context.Background(), ctrlclient.ObjectKey{
		Name:      "my-func",
		Namespace: "default",
	}, &fn)
	if err != nil {
		t.Fatal(err)
	}
	ts, ok := fn.Annotations["functions.knative.dev/last-deployed"]
	if !ok {
		t.Fatal("expected last-deployed annotation to be set")
	}
	if _, err := time.Parse(time.RFC3339, ts); err != nil {
		t.Fatalf("expected valid RFC3339 timestamp, got %q: %v", ts, err)
	}
	if fn.Spec.Repository.URL != "https://github.com/alice/my-func.git" {
		t.Fatalf("expected spec to be updated, but URL was %q", fn.Spec.Repository.URL)
	}
	if fn.Spec.Repository.Revision != "main" {
		t.Fatalf("expected revision to be updated, but got %q", fn.Spec.Repository.Revision)
	}
}

func TestSyncFunctionCR_UpdateExistingByStatusName(t *testing.T) {
	scheme := newScheme()
	existing := &v1alpha1.Function{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "different-cr-name",
			Namespace: "default",
		},
		Spec: v1alpha1.FunctionSpec{
			Repository: v1alpha1.FunctionSpecRepository{
				URL: "https://github.com/old/repo.git",
			},
		},
		Status: v1alpha1.FunctionStatus{
			Name: "my-func",
		},
	}
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		WithStatusSubresource(existing).
		Build()
	disc := fakeDiscoveryWithCRD()

	cfg := SyncConfig{
		FunctionName: "my-func",
		Namespace:    "default",
		RepoURL:      "https://github.com/alice/my-func.git",
		RepoRevision: "main",
		RepoDir:      ".",
	}

	err := syncFunctionCR(context.Background(), cl, disc, cfg)
	if err != nil {
		t.Fatal(err)
	}

	var fn v1alpha1.Function
	err = cl.Get(context.Background(), ctrlclient.ObjectKey{
		Name:      "different-cr-name",
		Namespace: "default",
	}, &fn)
	if err != nil {
		t.Fatal(err)
	}
	ts, ok := fn.Annotations["functions.knative.dev/last-deployed"]
	if !ok {
		t.Fatal("expected last-deployed annotation to be set")
	}
	if _, err := time.Parse(time.RFC3339, ts); err != nil {
		t.Fatalf("expected valid RFC3339 timestamp, got %q: %v", ts, err)
	}
}

func TestSyncFunctionCR_NoCRD_SkipSilently(t *testing.T) {
	scheme := newScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	disc := fakeDiscoveryWithoutCRD()

	cfg := SyncConfig{
		FunctionName: "my-func",
		Namespace:    "default",
		RepoURL:      "https://github.com/alice/my-func.git",
		RepoRevision: "main",
		RepoDir:      ".",
	}

	err := syncFunctionCR(context.Background(), cl, disc, cfg)
	if err != nil {
		t.Fatalf("expected no error when CRD missing, got: %v", err)
	}
}

func TestSyncFunctionCR_WithRegistryCredentials(t *testing.T) {
	original := ensureRegistrySecret
	ensureRegistrySecret = func(_ context.Context, _, _ string, _, _ map[string]string, _, _, _ string) error {
		return nil
	}
	t.Cleanup(func() { ensureRegistrySecret = original })

	scheme := newScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	disc := fakeDiscoveryWithCRD()

	cfg := SyncConfig{
		FunctionName: "my-func",
		Namespace:    "default",
		RepoURL:      "https://github.com/alice/my-func.git",
		RepoRevision: "main",
		RepoDir:      ".",
		RegistryCredentials: &RegistryCredentials{
			Username: "admin",
			Password: "secret",
			Server:   "ghcr.io",
		},
	}

	err := syncFunctionCR(context.Background(), cl, disc, cfg)
	if err != nil {
		t.Fatal(err)
	}

	var fn v1alpha1.Function
	err = cl.Get(context.Background(), ctrlclient.ObjectKey{
		Name:      "my-func",
		Namespace: "default",
	}, &fn)
	if err != nil {
		t.Fatalf("expected Function CR to be created: %v", err)
	}
	if fn.Spec.Registry.AuthSecretRef == nil {
		t.Fatal("expected registry authSecretRef to be set")
	}
	if fn.Spec.Registry.AuthSecretRef.Name != "my-func-registry-auth" {
		t.Fatalf("expected secret name 'my-func-registry-auth', got %q", fn.Spec.Registry.AuthSecretRef.Name)
	}
}

func TestSyncFunctionCR_NoRepoURL_SkipsWithMessage(t *testing.T) {
	scheme := newScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	disc := fakeDiscoveryWithCRD()

	cfg := SyncConfig{
		FunctionName: "my-func",
		Namespace:    "default",
		RepoURL:      "",
		RepoRevision: "main",
		RepoDir:      ".",
	}

	err := syncFunctionCR(context.Background(), cl, disc, cfg)
	if err != nil {
		t.Fatalf("expected no error when repo URL empty, got: %v", err)
	}

	// Verify no CR was created
	var list v1alpha1.FunctionList
	if err := cl.List(context.Background(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 0 {
		t.Fatalf("expected no Function CRs, got %d", len(list.Items))
	}
}

// TestSyncFunctionCR_PrintsServerWarnings ensures a warning the API server
// returns for the CR, such as a field the cluster's CRD drops, is printed to
// stderr rather than lost.
func TestSyncFunctionCR_PrintsServerWarnings(t *testing.T) {
	const warning = `unknown field "spec.repository.branch"`
	gv := v1alpha1.GroupVersion.String()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api":
			_ = json.NewEncoder(w).Encode(metav1.APIVersions{})
		case r.URL.Path == "/apis":
			version := metav1.GroupVersionForDiscovery{GroupVersion: gv, Version: v1alpha1.GroupVersion.Version}
			_ = json.NewEncoder(w).Encode(metav1.APIGroupList{Groups: []metav1.APIGroup{{
				Name:             v1alpha1.GroupVersion.Group,
				Versions:         []metav1.GroupVersionForDiscovery{version},
				PreferredVersion: version,
			}}})
		case r.URL.Path == "/apis/"+gv:
			_ = json.NewEncoder(w).Encode(metav1.APIResourceList{
				GroupVersion: gv,
				APIResources: []metav1.APIResource{{Name: "functions", Kind: "Function", Namespaced: true}},
			})
		case r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"apiVersion":"`+gv+`","kind":"FunctionList","items":[]}`)
		case r.Method == http.MethodPost:
			w.Header().Set("Warning", "299 - "+strconv.Quote(warning))
			w.WriteHeader(http.StatusCreated)
			_, _ = io.Copy(w, r.Body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	kubeconfig := filepath.Join(t.TempDir(), "kubeconfig")
	err := os.WriteFile(kubeconfig, []byte(`apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: `+srv.URL+`
contexts:
- name: test
  context:
    cluster: test
current-context: test
`), 0600)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", kubeconfig)

	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err = SyncFunctionCR(context.Background(), SyncConfig{
		FunctionName: "my-func",
		Namespace:    "default",
		RepoURL:      "https://github.com/alice/my-func.git",
		RepoRevision: "main",
		RepoDir:      ".",
	})

	w.Close()
	os.Stderr = old
	if err != nil {
		t.Fatal(err)
	}

	output, _ := io.ReadAll(r)
	if !strings.Contains(string(output), "Warning: "+warning) {
		t.Fatalf("expected server warning on stderr, got:\n%s", output)
	}
}
