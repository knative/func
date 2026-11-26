package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const testKedaHTTPAddonManifest = `apiVersion: v1
kind: Service
metadata:
  name: keda-add-ons-http-interceptor-proxy
  namespace: keda
spec:
  type: ClusterIP
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: keda-add-ons-http-interceptor
  namespace: keda
spec:
  replicas: 3
  selector:
    matchLabels:
      app: interceptor
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: keda-add-ons-http-operator
  namespace: keda
spec:
  selector:
    matchLabels:
      app: operator
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: keda-add-ons-http-scaler
  namespace: keda
spec:
  replicas: 3
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: unrelated-deployment
  namespace: keda
spec:
  replicas: 5
---
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: keda-add-ons-http-interceptor
  namespace: keda
spec:
  minReplicaCount: 3
  maxReplicaCount: 50
  pollingInterval: 1
---
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: unrelated-scaledobject
  namespace: keda
spec:
  minReplicaCount: 4
`

type recordedKubectlCall struct {
	Args  []string `json:"args"`
	Stdin string   `json:"stdin"`
}

func TestMain(m *testing.M) {
	if logDir := os.Getenv("FUNC_TEST_FAKE_KUBECTL_DIR"); logDir != "" {
		os.Exit(runFakeKubectl(logDir, os.Args[1:]))
	}
	os.Exit(m.Run())
}

func runFakeKubectl(logDir string, args []string) int {
	var stdin string
	if info, err := os.Stdin.Stat(); err == nil && (info.Mode()&os.ModeCharDevice) == 0 {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "reading stdin: %v\n", err)
			return 1
		}
		stdin = string(b)
	}
	entries, err := os.ReadDir(logDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading log dir: %v\n", err)
		return 1
	}
	rec := recordedKubectlCall{Args: args, Stdin: stdin}
	data, err := json.Marshal(rec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshaling record: %v\n", err)
		return 1
	}
	path := filepath.Join(logDir, fmt.Sprintf("%03d.json", len(entries)))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "writing record: %v\n", err)
		return 1
	}
	return 0
}

func TestInstallKedaHTTPAddon(t *testing.T) {
	addonVersionNum := strings.TrimPrefix(kedaHTTPAddOnVersion, "v")
	crdsPath := fmt.Sprintf("/%s/keda-add-ons-http-%s-crds.yaml", kedaHTTPAddOnVersion, addonVersionNum)
	addonPath := fmt.Sprintf("/%s/keda-add-ons-http-%s.yaml", kedaHTTPAddOnVersion, addonVersionNum)
	crdsYAML := "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nmetadata:\n  name: httpscaledobjects.http.keda.sh\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case crdsPath:
			_, _ = io.WriteString(w, crdsYAML)
		case addonPath:
			_, _ = io.WriteString(w, testKedaHTTPAddonManifest)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	origBaseURL := kedaHTTPAddonReleaseBaseURL
	kedaHTTPAddonReleaseBaseURL = srv.URL
	t.Cleanup(func() { kedaHTTPAddonReleaseBaseURL = origBaseURL })

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	tests := []struct {
		name               string
		githubActions      bool
		wantInterceptorRep int64
		wantScalerRep      int64
		wantMinReplica     int64
	}{
		{
			name:               "CI applies single-replica manifest before wait with no post-hoc scale or patch",
			githubActions:      true,
			wantInterceptorRep: 1,
			wantScalerRep:      1,
			wantMinReplica:     1,
		},
		{
			name:               "non-CI applies upstream manifest unmodified",
			githubActions:      false,
			wantInterceptorRep: 3,
			wantScalerRep:      3,
			wantMinReplica:     3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logDir := t.TempDir()
			t.Setenv("FUNC_TEST_FAKE_KUBECTL_DIR", logDir)

			cfg := ClusterConfig{
				KubectlOverride: exe,
				GitHubActions:   tc.githubActions,
			}
			var out bytes.Buffer
			if err := installKedaHTTPAddon(context.Background(), cfg, &out); err != nil {
				t.Fatalf("installKedaHTTPAddon() error = %v\noutput:\n%s", err, out.String())
			}

			calls := readRecordedKubectlCalls(t, logDir)
			wantSeq := [][]string{
				{"apply", "--server-side", "-f", "-"},
				{"apply", "--server-side", "-f", "-"},
				{"wait", "deployment", "--all", "--timeout=-1s", "--for=condition=Available", "--namespace", "keda"},
				{"get", "pod", "-n", "keda"},
			}
			if len(calls) != len(wantSeq) {
				t.Fatalf("expected %d kubectl calls (apply crds, apply addon, wait, get pod), got %d: %+v", len(wantSeq), len(calls), calls)
			}
			for i, wantArgs := range wantSeq {
				if !reflect.DeepEqual(calls[i].Args, wantArgs) {
					t.Errorf("kubectl call %d args = %v, want %v", i, calls[i].Args, wantArgs)
				}
			}

			// Call 0 applies the CRDs manifest via server-side apply from stdin.
			crdDocs := decodeAllDocs(t, calls[0].Stdin)
			if len(crdDocs) != 1 || crdDocs[0].GetKind() != "CustomResourceDefinition" {
				t.Fatalf("expected 1 CRD doc in call 0, got %d", len(crdDocs))
			}

			// Call 1 applies the add-on manifest via server-side apply from stdin.
			docs := decodeAllDocs(t, calls[1].Stdin)
			if len(docs) != 7 {
				t.Fatalf("expected 7 applied documents, got %d", len(docs))
			}

			byKey := indexByKindName(docs)
			assertNestedInt64(t, byKey, "Deployment/keda-add-ons-http-interceptor", tc.wantInterceptorRep, "spec", "replicas")
			assertNestedInt64(t, byKey, "Deployment/keda-add-ons-http-scaler", tc.wantScalerRep, "spec", "replicas")
			assertNestedInt64(t, byKey, "ScaledObject/keda-add-ons-http-interceptor", tc.wantMinReplica, "spec", "minReplicaCount")
			assertNestedInt64(t, byKey, "ScaledObject/keda-add-ons-http-interceptor", 50, "spec", "maxReplicaCount")
			assertNestedInt64(t, byKey, "Deployment/unrelated-deployment", 5, "spec", "replicas")
			assertNestedInt64(t, byKey, "ScaledObject/unrelated-scaledobject", 4, "spec", "minReplicaCount")

			op := byKey["Deployment/keda-add-ons-http-operator"]
			if _, found, err := unstructured.NestedFieldNoCopy(op.Object, "spec", "replicas"); err != nil || found {
				t.Errorf("operator Deployment replicas found=%v err=%v, want not found", found, err)
			}
		})
	}
}

func TestReduceKedaHTTPAddonReplicas(t *testing.T) {
	got, err := reduceKedaHTTPAddonReplicas(testKedaHTTPAddonManifest)
	if err != nil {
		t.Fatalf("reduceKedaHTTPAddonReplicas() error = %v", err)
	}

	docs := decodeAllDocs(t, got)
	if len(docs) != 7 {
		t.Fatalf("expected 7 documents, got %d", len(docs))
	}

	byKey := indexByKindName(docs)
	assertNestedInt64(t, byKey, "Deployment/keda-add-ons-http-interceptor", 1, "spec", "replicas")
	assertNestedInt64(t, byKey, "Deployment/keda-add-ons-http-scaler", 1, "spec", "replicas")
	assertNestedInt64(t, byKey, "Deployment/unrelated-deployment", 5, "spec", "replicas")
	assertNestedInt64(t, byKey, "ScaledObject/keda-add-ons-http-interceptor", 1, "spec", "minReplicaCount")
	assertNestedInt64(t, byKey, "ScaledObject/keda-add-ons-http-interceptor", 50, "spec", "maxReplicaCount")
	assertNestedInt64(t, byKey, "ScaledObject/unrelated-scaledobject", 4, "spec", "minReplicaCount")

	// Operator Deployment has no replicas field in the upstream manifest and
	// must not have one added.
	op := byKey["Deployment/keda-add-ons-http-operator"]
	if _, found, err := unstructured.NestedFieldNoCopy(op.Object, "spec", "replicas"); err != nil || found {
		t.Errorf("operator Deployment replicas found=%v err=%v, want not found", found, err)
	}

	// Unrelated Service is preserved intact.
	svc, ok := byKey["Service/keda-add-ons-http-interceptor-proxy"]
	if !ok {
		t.Fatal("missing Service/keda-add-ons-http-interceptor-proxy")
	}
	if typ, _, _ := unstructured.NestedString(svc.Object, "spec", "type"); typ != "ClusterIP" {
		t.Errorf("Service spec.type = %q, want ClusterIP", typ)
	}
}

func TestReduceKedaHTTPAddonReplicas_EdgeCases(t *testing.T) {
	t.Run("leading separator, comments, and CRLF", func(t *testing.T) {
		input := "---\r\n# header comment only\r\n---\r\napiVersion: apps/v1\r\nkind: Deployment\r\nmetadata:\r\n  name: keda-add-ons-http-interceptor\r\nspec:\r\n  replicas: 3\r\n---\r\n"
		got, err := reduceKedaHTTPAddonReplicas(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		docs := decodeAllDocs(t, got)
		if len(docs) != 1 {
			t.Fatalf("expected 1 document, got %d", len(docs))
		}
		v, found, err := unstructured.NestedInt64(docs[0].Object, "spec", "replicas")
		if err != nil || !found || v != 1 {
			t.Errorf("replicas = (%d, found=%v, err=%v), want (1, true, nil)", v, found, err)
		}
	})

	t.Run("invalid YAML", func(t *testing.T) {
		if _, err := reduceKedaHTTPAddonReplicas("kind: [\ninvalid"); err == nil {
			t.Fatal("expected error for invalid YAML")
		}
	})

	t.Run("non-map spec on target Deployment", func(t *testing.T) {
		input := "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: keda-add-ons-http-scaler\nspec: invalid\n"
		if _, err := reduceKedaHTTPAddonReplicas(input); err == nil {
			t.Fatal("expected error for non-map spec on target Deployment")
		}
	})

	t.Run("non-map spec on target ScaledObject", func(t *testing.T) {
		input := "apiVersion: keda.sh/v1alpha1\nkind: ScaledObject\nmetadata:\n  name: keda-add-ons-http-interceptor\nspec: invalid\n"
		if _, err := reduceKedaHTTPAddonReplicas(input); err == nil {
			t.Fatal("expected error for non-map spec on target ScaledObject")
		}
	})
}

func readRecordedKubectlCalls(t *testing.T, dir string) []recordedKubectlCall {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	calls := make([]recordedKubectlCall, 0, len(names))
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", name, err)
		}
		var rec recordedKubectlCall
		if err := json.Unmarshal(data, &rec); err != nil {
			t.Fatalf("Unmarshal(%s): %v", name, err)
		}
		calls = append(calls, rec)
	}
	return calls
}

func indexByKindName(docs []unstructured.Unstructured) map[string]unstructured.Unstructured {
	m := make(map[string]unstructured.Unstructured, len(docs))
	for _, d := range docs {
		m[d.GetKind()+"/"+d.GetName()] = d
	}
	return m
}

func assertNestedInt64(t *testing.T, byKey map[string]unstructured.Unstructured, key string, want int64, fields ...string) {
	t.Helper()
	obj, ok := byKey[key]
	if !ok {
		t.Fatalf("missing document %s", key)
	}
	v, found, err := unstructured.NestedInt64(obj.Object, fields...)
	if err != nil {
		t.Fatalf("%s %v: %v", key, fields, err)
	}
	if !found || v != want {
		t.Errorf("%s %v = (%d, found=%v), want (%d, true)", key, fields, v, found, want)
	}
}

func decodeAllDocs(t *testing.T, content string) []unstructured.Unstructured {
	t.Helper()
	dec := yaml.NewYAMLOrJSONDecoder(strings.NewReader(content), 4096)
	var docs []unstructured.Unstructured
	for {
		var obj unstructured.Unstructured
		err := dec.Decode(&obj)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("decoding output stream: %v", err)
		}
		if obj.Object != nil {
			docs = append(docs, obj)
		}
	}
	return docs
}
