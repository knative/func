package knative

import (
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"

	fn "knative.dev/func/pkg/functions"
)

func Test_setHealthEndpoints(t *testing.T) {
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
	setHealthEndpoints(f, &c)
	got := c.LivenessProbe.HTTPGet.Path
	if got != "/lively" {
		t.Errorf("expected \"/lively\" but got %v", got)
	}
	got = c.ReadinessProbe.HTTPGet.Path
	if got != "/readyAsIllEverBe" {
		t.Errorf("expected \"readyAsIllEverBe\" but got %v", got)
	}
}

func Test_setHealthEndpointDefaults(t *testing.T) {
	f := fn.Function{
		Name: "testing",
	}
	c := corev1.Container{}
	setHealthEndpoints(f, &c)
	got := c.LivenessProbe.HTTPGet.Path
	if got != LIVENESS_ENDPOINT {
		t.Errorf("expected \"%v\" but got %v", LIVENESS_ENDPOINT, got)
	}
	got = c.ReadinessProbe.HTTPGet.Path
	if got != READINESS_ENDPOINT {
		t.Errorf("expected \"%v\" but got %v", READINESS_ENDPOINT, got)
	}
}

func Test_processValue(t *testing.T) {
	testEnvVarOld, testEnvVarOldExists := os.LookupEnv("TEST_KNATIVE_DEPLOYER")
	os.Setenv("TEST_KNATIVE_DEPLOYER", "VALUE_FOR_TEST_KNATIVE_DEPLOYER")
	defer func() {
		if testEnvVarOldExists {
			os.Setenv("TEST_KNATIVE_DEPLOYER", testEnvVarOld)
		} else {
			os.Unsetenv("TEST_KNATIVE_DEPLOYER")
		}
	}()

	unsetVarOld, unsetVarOldExists := os.LookupEnv("UNSET_VAR")
	os.Unsetenv("UNSET_VAR")
	defer func() {
		if unsetVarOldExists {
			os.Setenv("UNSET_VAR", unsetVarOld)
		}
	}()

	tests := []struct {
		name    string
		arg     string
		want    string
		wantErr bool
	}{
		{name: "simple value", arg: "A_VALUE", want: "A_VALUE", wantErr: false},
		{name: "using envvar value", arg: "{{ env:TEST_KNATIVE_DEPLOYER }}", want: "VALUE_FOR_TEST_KNATIVE_DEPLOYER", wantErr: false},
		{name: "bad context", arg: "{{secret:S}}", want: "", wantErr: true},
		{name: "unset envvar", arg: "{{env:SOME_UNSET_VAR}}", want: "", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := processLocalEnvValue(test.arg)
			if (err != nil) != test.wantErr {
				t.Errorf("processValue() error = %v, wantErr %v", err, test.wantErr)
				return
			}
			if got != test.want {
				t.Errorf("processValue() got = %v, want %v", got, test.want)
			}
		})
	}
}
