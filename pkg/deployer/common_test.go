package deployer

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	fn "knative.dev/func/pkg/functions"
)

func Test_SetHealthEndpoints(t *testing.T) {
	f := fn.Function{
		Name: "testing",
		Deploy: fn.DeploySpec{
			HealthEndpoints: fn.HealthEndpoints{
				Liveness:  "/lively",
				Readiness: "/readyAsIllEverBe",
			},
		},
	}
	c := corev1.Container{}
	SetHealthEndpoints(f, &c)
	got := c.LivenessProbe.HTTPGet.Path
	if got != "/lively" {
		t.Errorf("expected \"/lively\" but got %v", got)
	}
	got = c.ReadinessProbe.HTTPGet.Path
	if got != "/readyAsIllEverBe" {
		t.Errorf("expected \"readyAsIllEverBe\" but got %v", got)
	}
}

func Test_SetHealthEndpointDefaults(t *testing.T) {
	f := fn.Function{
		Name: "testing",
	}
	c := corev1.Container{}
	SetHealthEndpoints(f, &c)
	got := c.LivenessProbe.HTTPGet.Path
	if got != DefaultLivenessEndpoint {
		t.Errorf("expected \"%v\" but got %v", DefaultLivenessEndpoint, got)
	}
	got = c.ReadinessProbe.HTTPGet.Path
	if got != DefaultReadinessEndpoint {
		t.Errorf("expected \"%v\" but got %v", DefaultReadinessEndpoint, got)
	}
}
