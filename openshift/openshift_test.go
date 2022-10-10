//go:build integration
// +build integration

package openshift_test

import (
	"net/http"

	"testing"

	fnhttp "knative.dev/func/http"
	"knative.dev/func/openshift"
)

func TestRoundTripper(t *testing.T) {
	if !openshift.IsOpenShift() {
		t.Skip("The cluster in not an instance of OpenShift.")
		return
	}

	transport := fnhttp.NewRoundTripper(openshift.WithOpenShiftServiceCA())
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
