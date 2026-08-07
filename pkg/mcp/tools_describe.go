package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	fn "knative.dev/func/pkg/functions"
)

var describeTool = &mcp.Tool{
	Name:        "describe",
	Title:       "Describe Function",
	Description: "Describe a deployed Function: URL, image, namespace, labels, readiness, and event subscriptions.",
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

	out, err := s.executor.Execute(ctx, "describe", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}

	var instance fn.Instance
	if err = json.Unmarshal(out, &instance); err != nil {
		err = fmt.Errorf("failed to parse describe output: %w\n%s", err, string(out))
		return
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
		Revision:      instance.Revision,
	}
	return
}

// DescribeInput defines the input parameters for the describe tool.
// Exactly one of Path or Name must be provided.
type DescribeInput struct {
	Path      *string `json:"path,omitempty" jsonschema:"Path to the function project directory (mutually exclusive with name)"`
	Name      *string `json:"name,omitempty" jsonschema:"Name of the function to describe (mutually exclusive with path)"`
	Namespace *string `json:"namespace,omitempty" jsonschema:"Kubernetes namespace to describe from (default: current or active namespace)"`
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
	Revision      string            `json:"revision,omitempty" jsonschema:"Source commit SHA embedded in the image"`
}
