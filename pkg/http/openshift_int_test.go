//go:build integration

package http_test

import (
	"net/http"
	"testing"

	fn "knative.dev/func/pkg/functions"
	fnhttp "knative.dev/func/pkg/http"
	"knative.dev/func/pkg/k8s"
)

func TestInt_RoundTripper(t *testing.T) {
	cc, _ := k8s.BuildClientConfig("", "", "", fn.Local{})
	kc := k8s.NewClient(cc)
	if !kc.IsOpenshift() {
		t.Skip("The cluster in not an instance of OpenShift.")
		return
	}

	transport := fnhttp.NewRoundTripper(kc, fnhttp.WithOpenShiftServiceCA(kc))
	defer transport.Close()

	client := http.Client{
		Transport: transport,
	}

	resp, err := client.Get("https://image-registry.openshift-image-registry.svc.cluster.local:5000/v2/")
	if err != nil {
		t.Error(err)
		return
	}
	defer resp.Body.Close()
}
