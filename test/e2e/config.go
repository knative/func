package e2e

import (
	"os"
	"strings"

	"knative.dev/func/pkg/openshift"
)

// Intended to provide setup configuration for E2E tests
const (
	defaultRegistry        = "localhost:50000/user"
	testTemplateRepository = "http://github.com/boson-project/test-templates.git" //nolint:varcheck,deadcode
)

var testRegistry = ""

func init() {
	// Setup test Registry.
	testRegistry = os.Getenv("E2E_REGISTRY_URL")
	if testRegistry == "" || testRegistry == "default" {
		if openshift.IsOpenShift() {
			testRegistry = openshift.GetDefaultRegistry()
		} else {
			testRegistry = defaultRegistry
		}
	}
}

// GetRegistry returns registry
func GetRegistry() string {
	return testRegistry
}

// GetFuncBinaryPath should return the Path of 'func' binary under test
func GetFuncBinaryPath() string {
	return getOsEnvOrDefault("E2E_FUNC_BIN_PATH", "")
}

// GetRuntime returns the runtime that should be tested.
func GetRuntime() string {
	return getOsEnvOrDefault("E2E_RUNTIME", "node")
}

// IsUseKnFunc indicates that tests should be run against "kn func" instead of "func" binary
func IsUseKnFunc() bool {
	return strings.EqualFold(os.Getenv("E2E_USE_KN_FUNC"), "true")
}

func getOsEnvOrDefault(env string, dflt string) string {
	e := os.Getenv(env)
	if e == "" {
		return dflt
	}
	return e
}
