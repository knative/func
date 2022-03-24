package k8s_test

import (
	"context"
	"testing"

	"knative.dev/kn-plugin-func/k8s"
	. "knative.dev/kn-plugin-func/testing"
)

func TestListConfigMapsNamesIfConnectedWrongKubeconfig(t *testing.T) {
	defer WithEnvVar(t, "KUBECONFIG", "/tmp/non-existent.config")()
	_, err := k8s.ListConfigMapsNamesIfConnected(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
}

func TestListConfigMapsNamesIfConnectedWrongKubernentesMaster(t *testing.T) {
	defer WithEnvVar(t, "KUBERNETES_MASTER", "/tmp/non-existent.config")()
	_, err := k8s.ListConfigMapsNamesIfConnected(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
}
