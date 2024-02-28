//go:build !integration
// +build !integration

package knative

import (
	"fmt"
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"

	fn "knative.dev/func/pkg/functions"
	. "knative.dev/func/pkg/testing"
)

// Test_DefaultNamespace ensures that if there is an active kubeconfig,
// that namespace will be returned as the default from the public
// DefaultNamespace accessor, empty string otherwise.
func Test_DefaultNamespace(t *testing.T) {
	// Update Kubeconfig to indicate the currently active namespace is:
	// "test-ns-deploy"
	t.Setenv("KUBECONFIG", fmt.Sprintf("%s/testdata/test_default_namespace", Cwd()))

	if ActiveNamespace() != "test-ns-deploy" {
		t.Fatalf("expected 'test-ns-deploy', got '%v'", ActiveNamespace())
	}
}

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

// Test_namespace ensures the namespace function returns the correct namespace
// to use for the next deployment.
func Test_namespace(t *testing.T) {
	// store path to test kubeconfig before changing working directory.
	testKubeconfigPath := fmt.Sprintf("%s/testdata/test_default_namespace", Cwd())

	tests := []struct {
		testName, requested, current, expected string
		active                                 bool
	}{
		{
			testName:  "Static default",
			requested: "",    // nothing requested (such as via the CLI)
			current:   "",    // no current namespace (undeployed)
			active:    false, // no active k8s context to choose from
			expected:  DefaultNamespace,
		}, {
			testName:  "Active k8s context",
			requested: "",
			current:   "",
			active:    true,             // use the test active k8s context
			expected:  "test-ns-deploy", // that is what is expected
		}, {
			testName:  "Currently deployed",
			requested: "",
			current:   "default", // currently deployed to "default"
			active:    true,      // use the test active k8s context
			expected:  "default", // redeploy should take precidence
		}, {
			testName:  "Move request",
			requested: "func",    // Requesting it be moved to "func"
			current:   "default", // currently deployed to "default"
			active:    true,      // use the test active k8s context
			expected:  "func",    // requested should take highest precidence
		},
	}
	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			_ = FromTempDirectory(t) // clear existing envs and cd tmp

			// Populate a Function with the test settings
			testname := test.testName
			f := fn.Function{Name: "test"}
			f.Namespace = test.requested
			f.Deploy.Namespace = test.current
			if test.active {
				t.Setenv("KUBECONFIG", testKubeconfigPath)
			}

			// Assert the correct namespace is evaluated as effective
			if namespace(f) != test.expected {
				t.Fatalf("%q expected namespace %q, got %q", testname, test.expected, namespace(f))
			}
		})
	}
}
