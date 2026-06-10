package mcp

import (
	"context"

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
		IdempotentHint:  true,
	},
}

func (s *Server) buildHandler(ctx context.Context, r *mcp.CallToolRequest, input BuildInput) (result *mcp.CallToolResult, output BuildOutput, err error) {
	svc, err := s.requireService()
	if err != nil {
		return
	}
	output, err = svc.Build(ctx, input)
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

// BuildOutput defines the structured output returned by the build tool.
type BuildOutput struct {
	Image   string `json:"image,omitempty" jsonschema:"The built image name"`
	Message string `json:"message" jsonschema:"Output message from func command"`
}
