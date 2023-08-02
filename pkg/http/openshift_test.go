//go:build integration
// +build integration

package http_test

import (
	"net/http"
	"testing"

	fnhttp "knative.dev/func/pkg/http"
	"knative.dev/func/pkg/k8s"
)

func TestRoundTripper(t *testing.T) {
	if !k8s.IsOpenShift() {
		t.Skip("The cluster in not an instance of OpenShift.")
		return
	}

	transport := fnhttp.NewRoundTripper(fnhttp.WithOpenShiftServiceCA())
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
