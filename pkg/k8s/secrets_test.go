package k8s_test

import (
	"testing"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

func TestListSecretsNamesIfConnectedWrongKubeconfig(t *testing.T) {
	t.Setenv("KUBECONFIG", "/tmp/non-existent.config")
	cc, _ := k8s.BuildClientConfig("", "", "", fn.Local{})
	kc := k8s.NewClient(cc)
	_, err := k8s.ListSecretsNamesIfConnected(t.Context(), kc, "")
	if err != nil {
		t.Fatal(err)
	}
}

func TestListSecretsNamesIfConnectedWrongKubernentesMaster(t *testing.T) {
	t.Setenv("KUBERNETES_MASTER", "/tmp/non-existent.config")
	cc, _ := k8s.BuildClientConfig("", "", "", fn.Local{})
	kc := k8s.NewClient(cc)
	_, err := k8s.ListSecretsNamesIfConnected(t.Context(), kc, "")
	if err != nil {
		t.Fatal(err)
	}
}
