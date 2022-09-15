//go:build integration
// +build integration

package openshift_test

import (
	"net/http"

	"testing"

	fnhttp "knative.dev/kn-plugin-func/http"
	"knative.dev/kn-plugin-func/openshift"
	. "knative.dev/kn-plugin-func/testing"
)

func TestRoundTripper(t *testing.T) {
	defer WithEnvVar(t, "SOCAT_IMAGE", "quay.io/boson/alpine-socat:1.7.4.3-r1-non-root")()

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
