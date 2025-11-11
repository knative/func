package testing

import (
	"context"
	testing2 "testing"

	"k8s.io/api/core/v1"
	v2 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"knative.dev/func/pkg/k8s"
)

const DefaultIntTestNamespacePrefix = "func-int-test"

// Namespace returns the integration test namespace or that specified by
// FUNC_INT_NAMESPACE (creating if necessary)
func Namespace(t *testing2.T, ctx context.Context) string {
	t.Helper()

	cliSet, err := k8s.NewKubernetesClientset()
	if err != nil {
		t.Fatal(err)
	}

	// TODO: choose FUNC_INT_NAMESPACE if it exists?

	namespace := DefaultIntTestNamespacePrefix + "-" + rand.String(5)

	ns := &v1.Namespace{
		ObjectMeta: v2.ObjectMeta{
			Name: namespace,
		},
		Spec: v1.NamespaceSpec{},
	}
	_, err = cliSet.CoreV1().Namespaces().Create(ctx, ns, v2.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		err := cliSet.CoreV1().Namespaces().Delete(context.Background(), namespace, v2.DeleteOptions{})
		if err != nil {
			t.Logf("error deleting namespace: %v", err)
		}
	})
	t.Log("created namespace: ", namespace)

	return namespace
}
