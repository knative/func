//go:build integration && openshift
// +build integration,openshift

package openshift_test

import (
	"net/http"
	"testing"

	"knative.dev/kn-plugin-func/openshift"
)

func TestRoundTripper(t *testing.T) {
	transport := openshift.NewRoundTripper()

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
