package functions

import (
	"strings"
	"testing"
)

// TestDeployResultMessage covers the deploy success message matrix:
// deployed/updated crossed with exposed ("exposed at URL") vs cluster-local
// ("reachable in-cluster only").
func TestDeployResultMessage(t *testing.T) {
	tests := []struct {
		name    string
		status  Status
		exposed bool
		want    string
	}{
		{"deployed and exposed", Deployed, true, "deployed in namespace \"ns\" and exposed at URL"},
		{"deployed, in-cluster only", Deployed, false, "deployed in namespace \"ns\", reachable in-cluster only"},
		{"updated and exposed", Updated, true, "updated in namespace \"ns\" and exposed at URL"},
		{"updated, in-cluster only", Updated, false, "updated in namespace \"ns\", reachable in-cluster only"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deployResultMessage(DeploymentResult{
				Status:    tt.status,
				Namespace: "ns",
				URL:       "http://f.example.com",
				Exposed:   tt.exposed,
			})
			if !strings.Contains(got, tt.want) || !strings.Contains(got, "http://f.example.com") {
				t.Errorf("expected message to contain %q and the URL, got: %q", tt.want, got)
			}
		})
	}
}
