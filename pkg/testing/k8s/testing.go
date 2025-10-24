// package testing includes Kubernetes-specific testing helpers.
package k8s

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"knative.dev/func/pkg/k8s"
)

const DefaultIntTestNamespacePrefix = "func-int-test"

// Namespace returns the integration test namespace or that specified by
// FUNC_INT_NAMESPACE (creating if necessary)
func Namespace(t *testing.T, ctx context.Context) string {
	t.Helper()

	cliSet, err := k8s.NewKubernetesClientset()
	if err != nil {
		t.Fatal(err)
	}

	// TODO: choose FUNC_INT_NAMESPACE if it exists?

	namespace := DefaultIntTestNamespacePrefix + "-" + rand.String(5)

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
		Spec: corev1.NamespaceSpec{},
	}
	_, err = cliSet.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		err := cliSet.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
		if err != nil {
			t.Logf("error deleting namespace: %v", err)
		}
	})
	t.Log("created namespace: ", namespace)

	return namespace
}
