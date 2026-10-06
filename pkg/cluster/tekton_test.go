package cluster

import (
	"strings"
	"testing"
)

// TestTektonRBACManifests_Keda ensures the deployer is granted access to the
// keda ScaledObject and TriggerAuthentication resources.
func TestTektonRBACManifests_Keda(t *testing.T) {
	all := strings.Join(tektonRBACManifests("tekton-pipelines"), "\n---\n")

	for _, want := range []string{
		"name: func-keda-deployer",
		"scaledobjects",
		"triggerauthentications",
		"name: tekton-pipelines:func-keda-deployer",
		"namespace: tekton-pipelines",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("rbac manifests missing %q", want)
		}
	}
}
