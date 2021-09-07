package e2e

import (
	"os"
	"strings"
)

// Intended to provide setup configuration for E2E tests
const (
	defaultRegistry        = "localhost:5000/user"
	testTemplateRepository = "http://github.com/boson-project/test-templates.git" //nolint:varcheck,deadcode
)

// GetRegistry returns registry
func GetRegistry() string {
	return getOsEnvOrDefault("E2E_REGISTRY_URL", defaultRegistry)
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
