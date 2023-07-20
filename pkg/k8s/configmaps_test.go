package k8s_test

import (
	"context"
	"testing"

	"knative.dev/func/pkg/k8s"
)

func TestListConfigMapsNamesIfConnectedWrongKubeconfig(t *testing.T) {
	t.Setenv("KUBECONFIG", "/tmp/non-existent.config")
	_, err := k8s.ListConfigMapsNamesIfConnected(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
}

func TestListConfigMapsNamesIfConnectedWrongKubernentesMaster(t *testing.T) {
	t.Setenv("KUBERNETES_MASTER", "/tmp/non-existent.config")
	_, err := k8s.ListConfigMapsNamesIfConnected(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
}
