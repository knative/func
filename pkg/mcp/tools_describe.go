package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	fn "knative.dev/func/pkg/functions"
)

var describeTool = &mcp.Tool{
	Name:        "describe",
	Title:       "Describe Function",
	Description: "Describe a deployed Function: URL, routes, image, namespace, deployer, labels, revision, readiness, and event subscriptions.",
	Annotations: &mcp.ToolAnnotations{
		Title:          "Describe Function",
		ReadOnlyHint:   true,
		IdempotentHint: true, // Describing the same function multiple times returns consistent results at any point in time.
	},
}

func (s *Server) describeHandler(ctx context.Context, r *mcp.CallToolRequest, input DescribeInput) (result *mcp.CallToolResult, output DescribeOutput, err error) {
	// Validate: exactly one of Path or Name must be provided
	if (input.Path != nil && input.Name != nil) || (input.Path == nil && input.Name == nil) {
		err = fmt.Errorf("exactly one of 'path' or 'name' must be provided")
		return
	}

	// Validate: namespace only makes sense alongside 'name'. When describing
	// by 'path', the Function's name and namespace are read from its own
	// deploy identity (func.yaml); the CLI rejects a separate --namespace in
	// that mode ("must also specify a name when specifying namespace").
	if input.Path != nil && input.Namespace != nil {
		err = fmt.Errorf("'namespace' is only valid with 'name'; when describing by 'path', the namespace is read from the Function's own deploy identity")
		return
	}

	// ExecuteSplit (rather than Execute/CombinedOutput) is required here:
	// the CLI can write warnings to stderr on an otherwise-successful call
	// (e.g. permission warnings from the knative describer), and stdout and
	// stderr copied via CombinedOutput have no guaranteed relative ordering.
	// Parsing JSON only ever out of a clean, unmixed stdout avoids that
	// entirely rather than relying on any heuristic about stream ordering.
	stdout, stderr, err := s.executor.ExecuteSplit(ctx, "describe", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\nstdout: %s\nstderr: %s", err, string(stdout), string(stderr))
		return
	}

	var instance fn.Instance
	if err = json.Unmarshal(stdout, &instance); err != nil {
		err = fmt.Errorf("failed to parse describe output: %w\n%s", err, string(stdout))
		return
	}

	var middleware *fn.Middleware
	if instance.Middleware.Version != "" {
		middleware = &instance.Middleware
	}

	output = DescribeOutput{
		Name:          instance.Name,
		Namespace:     instance.Namespace,
		URL:           instance.Route,
		Routes:        instance.Routes,
		Image:         instance.Image,
		Ready:         instance.Ready,
		Deployer:      instance.Deployer,
		Labels:        instance.Labels,
		Subscriptions: instance.Subscriptions,
		Middleware:    middleware,
		Revision:      instance.Revision,
		// A non-fatal warning on stderr (e.g. RBAC denying the eventing
		// trigger list) can accompany an otherwise-successful, but partial,
		// JSON payload on stdout (e.g. an empty subscriptions list).
		// Surface it so the agent doesn't mistake "no subscriptions" for
		// "no permission to see them".
		Warnings: strings.TrimSpace(string(stderr)),
	}
	return
}

// DescribeInput defines the input parameters for the describe tool.
// Exactly one of Path or Name must be provided.
type DescribeInput struct {
	Path      *string `json:"path,omitempty" jsonschema:"Path to the function project directory (mutually exclusive with name)"`
	Name      *string `json:"name,omitempty" jsonschema:"Name of the function to describe (mutually exclusive with path)"`
	Namespace *string `json:"namespace,omitempty" jsonschema:"Kubernetes namespace to describe from (default: current or active namespace). Only valid together with 'name'; when describing by 'path' the namespace is read from the Function's own deploy identity"`
	Verbose   *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i DescribeInput) Args() []string {
	args := []string{}

	// Either path flag or positional name argument
	if i.Path != nil {
		args = append(args, "--path", *i.Path)
	} else if i.Name != nil {
		args = append(args, *i.Name)
	}

	args = appendStringFlag(args, "--namespace", i.Namespace)
	args = appendBoolFlag(args, "--verbose", i.Verbose)

	// The tool's contract is structured JSON, regardless of caller input.
	args = append(args, "--output", "json")
	return args
}

// DescribeOutput defines the structured output returned by the describe tool.
type DescribeOutput struct {
	Name          string            `json:"name" jsonschema:"Function name"`
	Namespace     string            `json:"namespace,omitempty" jsonschema:"Kubernetes namespace"`
	URL           string            `json:"url,omitempty" jsonschema:"Primary route URL"`
	Routes        []string          `json:"routes,omitempty" jsonschema:"All route URLs"`
	Image         string            `json:"image,omitempty" jsonschema:"Deployed container image"`
	Ready         string            `json:"ready,omitempty" jsonschema:"Overall readiness (true/false/unknown)"`
	Deployer      string            `json:"deployer,omitempty" jsonschema:"Deployer backend (knative, k8s, keda)"`
	Labels        map[string]string `json:"labels,omitempty" jsonschema:"Function labels"`
	Subscriptions []fn.Subscription `json:"subscriptions,omitempty" jsonschema:"Active event subscriptions"`
	Middleware    *fn.Middleware    `json:"middleware,omitempty" jsonschema:"Middleware backend (e.g. keda) applied at deploy time, if any"`
	Revision      string            `json:"revision,omitempty" jsonschema:"Source commit SHA, read from the OCI revision label baked into the built image"`
	Warnings      string            `json:"warnings,omitempty" jsonschema:"Non-fatal warnings emitted while gathering this Function's status (e.g. permission errors that caused partial data, such as an empty subscriptions list)"`
}
