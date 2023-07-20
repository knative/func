package k8s_test

import (
	"context"
	"testing"

	"knative.dev/func/pkg/k8s"
)

func TestListSecretsNamesIfConnectedWrongKubeconfig(t *testing.T) {
	t.Setenv("KUBECONFIG", "/tmp/non-existent.config")
	_, err := k8s.ListSecretsNamesIfConnected(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
}

func TestListSecretsNamesIfConnectedWrongKubernentesMaster(t *testing.T) {
	t.Setenv("KUBERNETES_MASTER", "/tmp/non-existent.config")
	_, err := k8s.ListSecretsNamesIfConnected(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
}
