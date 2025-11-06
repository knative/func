package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var buildTool = &mcp.Tool{
	Name:        "build",
	Title:       "Build Function",
	Description: "Build a Function's container image.",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Build Function",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(false),
		IdempotentHint:  true, // Building the same source code multiple times produces the same container image.
	},
}

func (s *Server) buildHandler(ctx context.Context, r *mcp.CallToolRequest, input BuildInput) (result *mcp.CallToolResult, output BuildOutput, err error) {
	out, err := s.executor.Execute(ctx, "build", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = BuildOutput{
		Message: string(out),
	}
	return
}

// BuildInput defines the input parameters for the build tool.
type BuildInput struct {
	Path             string  `json:"path" jsonschema:"required,Path to the function project directory"`
	Builder          *string `json:"builder,omitempty" jsonschema:"Builder to use (pack, s2i, or host)"`
	Registry         *string `json:"registry,omitempty" jsonschema:"Container registry for function image"`
	BuilderImage     *string `json:"builderImage,omitempty" jsonschema:"Custom builder image to use with buildpacks"`
	Image            *string `json:"image,omitempty" jsonschema:"Full image name (overrides registry)"`
	Platform         *string `json:"platform,omitempty" jsonschema:"Target platform (e.g., linux/amd64)"`
	Push             *bool   `json:"push,omitempty" jsonschema:"Push image to registry after building"`
	RegistryInsecure *bool   `json:"registryInsecure,omitempty" jsonschema:"Skip TLS verification for insecure registries"`
	BuildTimestamp   *bool   `json:"buildTimestamp,omitempty" jsonschema:"Use actual time for image timestamp (buildpacks only)"`
	Verbose          *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i BuildInput) Args() []string {
	// Required
	args := []string{"--path", i.Path}

	// String flags
	args = appendStringFlag(args, "--builder", i.Builder)
	args = appendStringFlag(args, "--registry", i.Registry)
	args = appendStringFlag(args, "--builder-image", i.BuilderImage)
	args = appendStringFlag(args, "--image", i.Image)
	args = appendStringFlag(args, "--platform", i.Platform)

	// Boolean flags
	args = appendBoolFlag(args, "--push", i.Push)
	args = appendBoolFlag(args, "--registry-insecure", i.RegistryInsecure)
	args = appendBoolFlag(args, "--build-timestamp", i.BuildTimestamp)
	args = appendBoolFlag(args, "--verbose", i.Verbose)

	return args
}

// BuildOutput defines the structured output returned by the build tool.
type BuildOutput struct {
	Image   string `json:"image,omitempty" jsonschema:"The built image name"`
	Message string `json:"message" jsonschema:"Output message from func command"`
}
