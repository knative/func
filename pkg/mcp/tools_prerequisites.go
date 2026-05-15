package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
)

var checkPrerequisitesTool = &mcp.Tool{
	Name:        "check_prerequisites",
	Title:       "Check Prerequisites",
	Description: "Check if the environment is ready for Function development (Docker, Kubernetes, Knative, Registry).",
	Annotations: &mcp.ToolAnnotations{
		Title:          "Check Prerequisites",
		ReadOnlyHint:   true,
		IdempotentHint: true,
	},
}

func (s *Server) checkPrerequisitesHandler(ctx context.Context, r *mcp.CallToolRequest, input CheckPrerequisitesInput) (result *mcp.CallToolResult, output CheckPrerequisitesOutput, err error) {
	output.Checks = []CheckDetail{}
	output.Ready = true

	// 1. Check func binary version
	s.runCheck(ctx, &output, "func", "Check if func CLI is installed", []string{"version"})

	// 2. Check Docker/Podman daemon
	s.runCheck(ctx, &output, "docker", "Check if Docker/Podman daemon is running", []string{"!docker", "info"})

	// 3. Check Kubectl and Cluster connectivity
	s.runCheck(ctx, &output, "cluster", "Check Kubernetes cluster connectivity", []string{"!kubectl", "cluster-info"})

	// 4. Check Knative Serving CRDs
	s.runCheck(ctx, &output, "knative", "Check if Knative Serving is installed", []string{"!kubectl", "get", "crd", "services.serving.knative.dev"})

	// 5. Check Registry Configuration
	s.runRegistryCheck(&output)

	return
}

// runCheck is a helper to execute a check and update the output.
// args starting with "!" are executed directly, others use the func prefix.
func (s *Server) runCheck(ctx context.Context, output *CheckPrerequisitesOutput, name, description string, args []string) {
	var out []byte
	var err error

	if strings.HasPrefix(args[0], "!") {
		// Run external command directly (e.g. !kubectl)
		cmd := args[0][1:]
		out, err = s.executor.ExecuteRaw(ctx, cmd, args[1:]...)
	} else {
		// Run func subcommand
		out, err = s.executor.Execute(ctx, args[0], args[1:]...)
	}

	detail := CheckDetail{
		Component:   name,
		Description: description,
		Status:      "ok",
		Message:     strings.TrimSpace(string(out)),
	}

	if err != nil {
		output.Ready = false
		detail.Status = "error"
		detail.Message = fmt.Sprintf("Error: %v\n%s", err, strings.TrimSpace(string(out)))
		detail.Guidance = getGuidance(name)
	}

	output.Checks = append(output.Checks, detail)
}

// runRegistryCheck validates that a container registry is configured
// by reading the current Function's func.yaml or the global config.
func (s *Server) runRegistryCheck(output *CheckPrerequisitesOutput) {
	detail := CheckDetail{
		Component:   "registry",
		Description: "Check if a container registry is configured",
		Status:      "ok",
	}

	// First, attempt to read the Function in the current directory
	f, err := fn.NewFunction("")
	if err == nil && f.Initialized() && f.Registry != "" {
		detail.Message = fmt.Sprintf("Registry configured: %s", f.Registry)
		output.Checks = append(output.Checks, detail)
		return
	}

	// No local func.yaml or no registry in it; check global config
	cfg, cfgErr := config.NewDefault()
	if cfgErr == nil && cfg.Registry != "" {
		detail.Message = fmt.Sprintf("Registry configured (global): %s", cfg.Registry)
		output.Checks = append(output.Checks, detail)
		return
	}

	// Neither local nor global config has a registry
	detail.Status = "warning"
	detail.Message = "No container registry configured in func.yaml or global config."
	detail.Guidance = getGuidance("registry")
	output.Checks = append(output.Checks, detail)
}

func getGuidance(component string) string {
	switch component {
	case "docker":
		return "Ensure Docker Desktop or Podman is running. If using Linux, check 'systemctl status docker'."
	case "cluster":
		return "Verify your ~/.kube/config is valid and you have a running Kubernetes cluster (e.g. kind, minikube)."
	case "knative":
		return "Knative Serving is not detected. Please install Knative or use a cluster with Knative pre-installed."
	case "func":
		return "Ensure the 'func' binary is in your PATH."
	case "registry":
		return "Set a default registry with 'func config registry' or pass --registry to build/deploy."
	default:
		return ""
	}
}

type CheckPrerequisitesInput struct {
	Verbose *bool `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

type CheckPrerequisitesOutput struct {
	Ready  bool          `json:"ready" jsonschema:"Whether all critical prerequisites are met"`
	Checks []CheckDetail `json:"checks" jsonschema:"Detailed status of each prerequisite check"`
}

type CheckDetail struct {
	Component   string `json:"component" jsonschema:"The component being checked (func, docker, cluster, knative, registry)"`
	Description string `json:"description" jsonschema:"Description of what this check does"`
	Status      string `json:"status" jsonschema:"Status of the check (ok, error, warning)"`
	Message     string `json:"message" jsonschema:"Output message from the check command"`
	Guidance    string `json:"guidance,omitempty" jsonschema:"Suggested actions to fix any errors"`
}

func (o CheckPrerequisitesOutput) String() string {
	b, _ := json.MarshalIndent(o, "", "  ")
	return string(b)
}
