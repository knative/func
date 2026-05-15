package mcp

import (
	"encoding/json"
	"strings"
)

// ErrorCategory classifies MCP tool failures into high-level categories that
// an AI agent can use to provide targeted remediation advice rather than
// surfacing raw CLI output.
//
// Example agent reasoning enabled by this:
//
//	"The error category is REGISTRY_ERROR. I should suggest the user
//	 check their registry credentials or run 'docker login' first."
type ErrorCategory string

const (
	// RegistryError indicates a failure related to the container registry,
	// such as authentication failure, push errors, or unreachable registry.
	RegistryError ErrorCategory = "REGISTRY_ERROR"

	// ClusterError indicates a failure in communicating with the Kubernetes
	// cluster, such as invalid kubeconfig, expired credentials, or cluster
	// being unreachable.
	ClusterError ErrorCategory = "CLUSTER_ERROR"

	// BuildError indicates a failure during the function build step,
	// such as a missing Dockerfile, buildpack error, or OOM condition.
	BuildError ErrorCategory = "BUILD_ERROR"

	// ValidationError indicates that the user's input or local configuration
	// is invalid, such as a missing func.yaml, bad path, or unknown runtime.
	ValidationError ErrorCategory = "VALIDATION_ERROR"

	// AuthError indicates a permissions or authentication-related failure,
	// such as RBAC denial on the cluster or unauthenticated registry access.
	AuthError ErrorCategory = "AUTH_ERROR"

	// UnknownError is the catch-all for failures that cannot be classified
	// into a more specific category.
	UnknownError ErrorCategory = "UNKNOWN_ERROR"
)

// StructuredError is a machine-readable error response returned by MCP tool
// handlers when a CLI operation fails. It extends the plain error message
// with a category that agents can use for targeted remediation.
type StructuredError struct {
	// Category is the high-level classification of the failure.
	Category ErrorCategory `json:"errorCategory"`

	// Message contains the full error output from the CLI command.
	Message string `json:"message"`

	// Hint is an optional human-readable suggestion for the agent to relay
	// to the user. It is set automatically based on the Category.
	Hint string `json:"hint,omitempty"`
}

// Error implements the error interface so StructuredError can be used
// as a standard Go error.
func (e *StructuredError) Error() string {
	b, _ := json.Marshal(e)
	return string(b)
}

// categorizeError inspects CLI output and classifies the error into the most
// appropriate ErrorCategory. It is intentionally heuristic — it scans for
// well-known substrings from common tool failures.
func categorizeError(output string) (ErrorCategory, string) {
	lower := strings.ToLower(output)

	switch {
	// Auth / RBAC errors — check FIRST as these strings overlap with others
	case containsAny(lower,
		"forbidden",
		"rbac",
		"is not allowed to",
		"permission denied",
		"cannot get",
		"cannot create",
		"cannot update"):
		return AuthError,
			"You may lack the required Kubernetes RBAC permissions. Contact your cluster administrator."

	// Registry / image push errors
	case containsAny(lower,
		"unauthorized", "authentication required",
		"denied", "access denied",
		"push", "pull", "manifest unknown",
		"failed to push", "failed to pull",
		"registry"):
		return RegistryError,
			"Check your registry credentials. Try running 'docker login <registry>' first."

	// Cluster / kubectl errors
	case containsAny(lower,
		"connection refused", "no such host",
		"cluster unreachable", "i/o timeout",
		"unable to connect to the server",
		"the server doesn't have a resource type",
		"error from server", "kubeconfig"):
		return ClusterError,
			"Check your Kubernetes context with 'kubectl cluster-info'. Ensure the cluster is reachable."

	// Build errors
	case containsAny(lower,
		"build failed", "buildpack",
		"error building", "failed to build",
		"no such file or directory",
		"exit status 1", "compilation failed"):
		return BuildError,
			"Check your function source code and dependencies. Review the build logs above for details."

	// Validation / config errors
	case containsAny(lower,
		"func.yaml", "not a function",
		"invalid", "not found",
		"missing required", "unknown runtime",
		"no function found"):
		return ValidationError,
			"Ensure you are running this command from a valid function directory containing a func.yaml file."

	default:
		return UnknownError,
			"An unexpected error occurred. Check the message above and retry, or run with --verbose for more details."
	}
}


// newStructuredError creates a StructuredError by automatically classifying
// the provided CLI output string. If output is empty, it uses the provided
// error's string representation.
func newStructuredError(output string, err error) *StructuredError {
	message := strings.TrimSpace(output)
	if message == "" && err != nil {
		message = err.Error()
	}

	cat, hint := categorizeError(message)
	return &StructuredError{
		Category: cat,
		Message:  message,
		Hint:     hint,
	}
}


// containsAny returns true if s contains any of the provided substrings.
func containsAny(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
