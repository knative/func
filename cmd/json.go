package cmd

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/spf13/cobra"
	"knative.dev/func/pkg/docker"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/oci"
)

const jsonAPIVersion = "v1"

// isJSONEnabled reports whether --json was explicitly set for this execution.
// Using cmd.Flag("json").Changed (rather than viper.GetBool("json")) avoids
// stale viper state polluting test runs.
func isJSONEnabled(cmd *cobra.Command) bool {
	f := cmd.Flag("json")
	return f != nil && f.Changed
}

// JSONResponse is the top-level envelope for all --json output.
type JSONResponse struct {
	APIVersion string     `json:"apiVersion"`
	Status     string     `json:"status"` // "ok" or "error"
	Data       any        `json:"data,omitempty"`
	Error      *JSONError `json:"error,omitempty"`
}

// JSONError carries structured failure information for machine consumers.
type JSONError struct {
	Category  string            `json:"category"`
	Code      string            `json:"code"`
	Retryable bool              `json:"retryable"`
	Message   string            `json:"message"`
	Hint      string            `json:"hint,omitempty"`
	Context   map[string]string `json:"context,omitempty"`
}

// WriteJSONSuccess writes a success envelope containing data to w.
// Exported for use in tests and pkg/app.
func WriteJSONSuccess(w io.Writer, data any) error {
	return json.NewEncoder(w).Encode(JSONResponse{
		APIVersion: jsonAPIVersion,
		Status:     "ok",
		Data:       data,
	})
}

// writeJSONSuccess is the package-internal alias.
func writeJSONSuccess(w io.Writer, data any) error {
	return WriteJSONSuccess(w, data)
}

// WriteJSONError classifies err and writes an error envelope to w.
// Exported so that pkg/app can call it from the top-level error sink.
func WriteJSONError(w io.Writer, err error) error {
	return json.NewEncoder(w).Encode(JSONResponse{
		APIVersion: jsonAPIVersion,
		Status:     "error",
		Error:      errorToJSONError(err),
	})
}

// writeJSONError is the package-internal alias.
func writeJSONError(w io.Writer, err error) error {
	return WriteJSONError(w, err)
}

// errorToJSONError maps a Go error to a structured JSONError by inspecting
// the known typed errors in the cmd and pkg/functions layers.
func errorToJSONError(err error) *JSONError {
	if err == nil {
		return nil
	}

	// --- CLUSTER errors ---

	var clusterNotAccessible *ErrClusterNotAccessible
	if errors.As(err, &clusterNotAccessible) {
		return &JSONError{
			Category:  "CLUSTER_ERROR",
			Code:      "CLUSTER_NOT_ACCESSIBLE",
			Retryable: true,
			Message:   clusterNotAccessible.Err.Error(),
			Hint:      "Verify your cluster is running: kubectl cluster-info",
		}
	}

	var listClusterConn *ErrListClusterConnection
	if errors.As(err, &listClusterConn) {
		return &JSONError{
			Category:  "CLUSTER_ERROR",
			Code:      "CLUSTER_NOT_ACCESSIBLE",
			Retryable: true,
			Message:   listClusterConn.Err.Error(),
			Hint:      "Verify your cluster is running: kubectl cluster-info",
		}
	}

	var invalidKubeconfig *ErrInvalidKubeconfig
	if errors.As(err, &invalidKubeconfig) {
		return &JSONError{
			Category:  "CLUSTER_ERROR",
			Code:      "INVALID_KUBECONFIG",
			Retryable: false,
			Message:   invalidKubeconfig.Err.Error(),
			Hint:      "Check your KUBECONFIG environment variable or ~/.kube/config",
		}
	}

	// --- AUTH / REGISTRY errors ---

	var registryRequiredCLI *ErrRegistryRequired
	if errors.As(err, &registryRequiredCLI) {
		return &JSONError{
			Category:  "VALIDATION_ERROR",
			Code:      "REGISTRY_REQUIRED",
			Retryable: false,
			Message:   registryRequiredCLI.Err.Error(),
			Hint:      "Provide --registry or set FUNC_REGISTRY",
			Context:   map[string]string{"command": registryRequiredCLI.Cmd},
		}
	}

	if errors.Is(err, fn.ErrRegistryRequired) {
		return &JSONError{
			Category:  "VALIDATION_ERROR",
			Code:      "REGISTRY_REQUIRED",
			Retryable: false,
			Message:   err.Error(),
			Hint:      "Provide --registry or set FUNC_REGISTRY",
		}
	}

	// --- VALIDATION errors ---

	var conflictImageRegistry *ErrConflictImageRegistry
	if errors.As(err, &conflictImageRegistry) {
		return &JSONError{
			Category:  "VALIDATION_ERROR",
			Code:      "CONFLICTING_IMAGE_REGISTRY",
			Retryable: false,
			Message:   conflictImageRegistry.Err.Error(),
			Hint:      "Use either --image or --registry, not both",
			Context:   map[string]string{"command": conflictImageRegistry.Cmd},
		}
	}

	var invalidNamespace *ErrInvalidNamespace
	if errors.As(err, &invalidNamespace) {
		return &JSONError{
			Category:  "VALIDATION_ERROR",
			Code:      "INVALID_NAMESPACE",
			Retryable: false,
			Message:   invalidNamespace.Err.Error(),
			Hint:      "Namespace must be lowercase alphanumeric and hyphens only, max 63 chars",
			Context:   map[string]string{"command": invalidNamespace.Cmd},
		}
	}

	var invalidDomain *ErrInvalidDomain
	if errors.As(err, &invalidDomain) {
		return &JSONError{
			Category:  "VALIDATION_ERROR",
			Code:      "INVALID_DOMAIN",
			Retryable: false,
			Message:   invalidDomain.Err.Error(),
			Hint:      "Domain must be a valid DNS subdomain",
			Context:   map[string]string{"command": invalidDomain.Cmd},
		}
	}

	var platformNotSupported *ErrPlatformNotSupported
	if errors.As(err, &platformNotSupported) {
		return &JSONError{
			Category:  "VALIDATION_ERROR",
			Code:      "PLATFORM_NOT_SUPPORTED",
			Retryable: false,
			Message:   platformNotSupported.Err.Error(),
			Hint:      "--platform is only supported with s2i and pack builders",
			Context:   map[string]string{"command": platformNotSupported.Cmd},
		}
	}

	var notInitializedCLI *ErrNotInitialized
	if errors.As(err, &notInitializedCLI) {
		ctx := map[string]string{}
		if notInitializedCLI.Cmd != "" {
			ctx["command"] = notInitializedCLI.Cmd
		}
		return &JSONError{
			Category:  "VALIDATION_ERROR",
			Code:      "NOT_INITIALIZED",
			Retryable: false,
			Message:   notInitializedCLI.Err.Error(),
			Hint:      "Run 'func create' to initialize a function first",
			Context:   ctx,
		}
	}

	var notInitializedCore *fn.ErrNotInitialized
	if errors.As(err, &notInitializedCore) {
		ctx := map[string]string{}
		if notInitializedCore.Path != "" {
			ctx["path"] = notInitializedCore.Path
		}
		return &JSONError{
			Category:  "VALIDATION_ERROR",
			Code:      "NOT_INITIALIZED",
			Retryable: false,
			Message:   notInitializedCore.Error(),
			Hint:      "Run 'func create' to initialize a function first",
			Context:   ctx,
		}
	}

	var deleteNameRequired *ErrDeleteNameRequired
	if errors.As(err, &deleteNameRequired) {
		return &JSONError{
			Category:  "VALIDATION_ERROR",
			Code:      "NAME_REQUIRED",
			Retryable: false,
			Message:   deleteNameRequired.Err.Error(),
			Hint:      "Provide a function name or use --path",
		}
	}

	var deleteNamespaceRequired *ErrDeleteNamespaceRequired
	if errors.As(err, &deleteNamespaceRequired) {
		return &JSONError{
			Category:  "VALIDATION_ERROR",
			Code:      "NAMESPACE_REQUIRED",
			Retryable: false,
			Message:   deleteNamespaceRequired.Err.Error(),
			Hint:      "Provide --namespace or use --path to a function with a recorded namespace",
		}
	}

	if errors.Is(err, fn.ErrNameRequired) {
		return &JSONError{
			Category:  "VALIDATION_ERROR",
			Code:      "NAME_REQUIRED",
			Retryable: false,
			Message:   err.Error(),
		}
	}

	if errors.Is(err, fn.ErrNamespaceRequired) {
		return &JSONError{
			Category:  "VALIDATION_ERROR",
			Code:      "NAMESPACE_REQUIRED",
			Retryable: false,
			Message:   err.Error(),
		}
	}

	// --- PORT / RUN errors ---

	var portPermissionDenied *ErrPortPermissionDenied
	if errors.As(err, &portPermissionDenied) {
		return &JSONError{
			Category:  "VALIDATION_ERROR",
			Code:      "PORT_PERMISSION_DENIED",
			Retryable: false,
			Message:   portPermissionDenied.Error(),
			Hint:      "Use a non-privileged port (>1024) or run with elevated permissions",
			Context:   map[string]string{"port": portPermissionDenied.Port},
		}
	}

	var portUnavailable *ErrPortUnavailable
	if errors.As(err, &portUnavailable) {
		return &JSONError{
			Category:  "VALIDATION_ERROR",
			Code:      "PORT_UNAVAILABLE",
			Retryable: true,
			Message:   portUnavailable.Err.Error(),
			Hint:      "Try a different port with --address",
			Context:   map[string]string{"port": portUnavailable.Port},
		}
	}

	// --- TEMPLATE / REPOSITORY errors ---

	if errors.Is(err, fn.ErrTemplateNotFound) {
		return &JSONError{
			Category:  "TEMPLATE_ERROR",
			Code:      "TEMPLATE_NOT_FOUND",
			Retryable: false,
			Message:   err.Error(),
			Hint:      "Run 'func repository list' to see available repositories and templates",
		}
	}

	if errors.Is(err, fn.ErrTemplatesNotFound) {
		return &JSONError{
			Category:  "TEMPLATE_ERROR",
			Code:      "TEMPLATES_NOT_FOUND",
			Retryable: false,
			Message:   err.Error(),
			Hint:      "The repository may be missing a 'templates' directory",
		}
	}

	if errors.Is(err, fn.ErrTemplateMissingRepository) {
		return &JSONError{
			Category:  "TEMPLATE_ERROR",
			Code:      "TEMPLATE_MISSING_REPOSITORY",
			Retryable: false,
			Message:   err.Error(),
			Hint:      "Specify the repository prefix, e.g. 'myrepo/http'",
		}
	}

	if errors.Is(err, fn.ErrRepositoryNotFound) {
		return &JSONError{
			Category:  "TEMPLATE_ERROR",
			Code:      "REPOSITORY_NOT_FOUND",
			Retryable: false,
			Message:   err.Error(),
			Hint:      "Add the repository with 'func repository add'",
		}
	}

	if errors.Is(err, fn.ErrRepositoriesNotDefined) {
		return &JSONError{
			Category:  "TEMPLATE_ERROR",
			Code:      "REPOSITORIES_NOT_DEFINED",
			Retryable: false,
			Message:   err.Error(),
			Hint:      "Set FUNC_REPOSITORIES_PATH or add a repository with 'func repository add'",
		}
	}

	// --- RUNTIME errors ---

	if errors.Is(err, fn.ErrRuntimeNotFound) {
		return &JSONError{
			Category:  "RUNTIME_ERROR",
			Code:      "RUNTIME_NOT_FOUND",
			Retryable: false,
			Message:   err.Error(),
			Hint:      "Run 'func languages' to see supported runtimes",
		}
	}

	if errors.Is(err, fn.ErrRuntimeRequired) {
		return &JSONError{
			Category:  "RUNTIME_ERROR",
			Code:      "RUNTIME_REQUIRED",
			Retryable: false,
			Message:   err.Error(),
			Hint:      "Provide --language or set the runtime in func.yaml",
		}
	}

	var runtimeNotRecognized fn.ErrRuntimeNotRecognized
	if errors.As(err, &runtimeNotRecognized) {
		return &JSONError{
			Category:  "RUNTIME_ERROR",
			Code:      "RUNTIME_NOT_RECOGNIZED",
			Retryable: false,
			Message:   runtimeNotRecognized.Error(),
			Hint:      "Run 'func languages' to see supported runtimes",
			Context:   map[string]string{"runtime": runtimeNotRecognized.Runtime},
		}
	}

	var runnerNotImplemented fn.ErrRunnerNotImplemented
	if errors.As(err, &runnerNotImplemented) {
		return &JSONError{
			Category:  "RUNTIME_ERROR",
			Code:      "RUNNER_NOT_IMPLEMENTED",
			Retryable: false,
			Message:   runnerNotImplemented.Error(),
			Hint:      "Use 'func deploy' to run containerized functions",
			Context:   map[string]string{"runtime": runnerNotImplemented.Runtime},
		}
	}

	var runTimeout fn.ErrRunTimeout
	if errors.As(err, &runTimeout) {
		return &JSONError{
			Category:  "RUNTIME_ERROR",
			Code:      "RUN_TIMEOUT",
			Retryable: true,
			Message:   runTimeout.Error(),
			Hint:      "The function did not become ready in time; check container logs",
		}
	}

	// --- FUNCTION state errors ---

	if errors.Is(err, fn.ErrFunctionNotFound) {
		return &JSONError{
			Category:  "NOT_FOUND",
			Code:      "FUNCTION_NOT_FOUND",
			Retryable: false,
			Message:   err.Error(),
		}
	}

	if errors.Is(err, fn.ErrNotRunning) {
		return &JSONError{
			Category:  "NOT_FOUND",
			Code:      "FUNCTION_NOT_RUNNING",
			Retryable: false,
			Message:   err.Error(),
			Hint:      "Start the function with 'func run' or 'func deploy'",
		}
	}

	// --- PORT / RUN errors (pkg/functions layer) ---

	var portUnavailableCore *fn.ErrPortUnavailableError
	if errors.As(err, &portUnavailableCore) {
		if portUnavailableCore.IsPermissionDenied() {
			return &JSONError{
				Category:  "VALIDATION_ERROR",
				Code:      "PORT_PERMISSION_DENIED",
				Retryable: false,
				Message:   portUnavailableCore.Error(),
				Hint:      "Use a non-privileged port (>1024) or run with elevated permissions",
				Context:   map[string]string{"port": portUnavailableCore.Port},
			}
		}
		return &JSONError{
			Category:  "VALIDATION_ERROR",
			Code:      "PORT_UNAVAILABLE",
			Retryable: true,
			Message:   portUnavailableCore.Error(),
			Hint:      "Try a different port with --address",
			Context:   map[string]string{"port": portUnavailableCore.Port},
		}
	}

	// --- BUILD errors ---

	if errors.Is(err, docker.ErrNoDocker) {
		return &JSONError{
			Category:  "BUILD_ERROR",
			Code:      "DOCKER_NOT_AVAILABLE",
			Retryable: true,
			Message:   err.Error(),
			Hint:      "Ensure Docker or Podman daemon is running",
		}
	}

	var buildErr oci.BuildErr
	if errors.As(err, &buildErr) {
		return &JSONError{
			Category:  "BUILD_ERROR",
			Code:      "BUILD_FAILED",
			Retryable: true,
			Message:   buildErr.Err.Error(),
		}
	}

	if errors.Is(err, fn.ErrNotBuilt) {
		return &JSONError{
			Category:  "BUILD_ERROR",
			Code:      "NOT_BUILT",
			Retryable: false,
			Message:   err.Error(),
			Hint:      "Run 'func build' before deploying",
		}
	}

	// --- fallback ---

	return &JSONError{
		Category:  "UNKNOWN_ERROR",
		Code:      "UNKNOWN",
		Retryable: false,
		Message:   err.Error(),
	}
}
