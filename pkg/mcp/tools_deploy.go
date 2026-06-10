package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var deployTool = &mcp.Tool{
	Name:        "deploy",
	Title:       "Deploy Function",
	Description: "Deploy a Function. Builds the container as needed.",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Deploy Function",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(false),
		IdempotentHint:  true,
	},
}

func (s *Server) deployHandler(ctx context.Context, r *mcp.CallToolRequest, input DeployInput) (result *mcp.CallToolResult, output DeployOutput, err error) {
	if s.readonly.Load() {
		err = fmt.Errorf("the server is currently in readonly mode.  Please set FUNC_ENABLE_MCP_WRITE and restart the client")
		return
	}
	svc, err := s.requireService()
	if err != nil {
		return
	}
	output, err = svc.Deploy(ctx, input)
	return
}

// DeployInput defines the input parameters for the deploy tool.
type DeployInput struct {
	Path               string  `json:"path" jsonschema:"required,Path to the function project directory"`
	Builder            *string `json:"builder,omitempty" jsonschema:"Builder to use (pack, s2i, or host)"`
	Registry           *string `json:"registry,omitempty" jsonschema:"Container registry for function image"`
	Image              *string `json:"image,omitempty" jsonschema:"Full image name (overrides registry)"`
	Namespace          *string `json:"namespace,omitempty" jsonschema:"Kubernetes namespace to deploy into"`
	GitURL             *string `json:"gitUrl,omitempty" jsonschema:"Git URL containing the function source"`
	GitBranch          *string `json:"gitBranch,omitempty" jsonschema:"Git branch for remote deployment"`
	GitDir             *string `json:"gitDir,omitempty" jsonschema:"Directory inside the Git repository"`
	BuilderImage       *string `json:"builderImage,omitempty" jsonschema:"Custom builder image"`
	Domain             *string `json:"domain,omitempty" jsonschema:"Domain for the function route"`
	Platform           *string `json:"platform,omitempty" jsonschema:"Target platform (e.g., linux/amd64)"`
	Build              *string `json:"build,omitempty" jsonschema:"Build control: true, false, or auto"`
	PVCSize            *string `json:"pvcSize,omitempty" jsonschema:"Custom volume size for remote builds"`
	ServiceAccount     *string `json:"serviceAccount,omitempty" jsonschema:"Kubernetes ServiceAccount to use"`
	RemoteStorageClass *string `json:"remoteStorageClass,omitempty" jsonschema:"Storage class for remote volume"`
	Push               *bool   `json:"push,omitempty" jsonschema:"Push image to registry before deployment"`
	RegistryInsecure   *bool   `json:"registryInsecure,omitempty" jsonschema:"Skip TLS verification for registry"`
	BuildTimestamp     *bool   `json:"buildTimestamp,omitempty" jsonschema:"Use actual time in image metadata"`
	Remote             *bool   `json:"remote,omitempty" jsonschema:"Trigger remote deployment"`
	Verbose            *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

// DeployOutput defines the structured output returned by the deploy tool.
type DeployOutput struct {
	URL     string `json:"url,omitempty" jsonschema:"The deployed Function URL"`
	Image   string `json:"image,omitempty" jsonschema:"The Function image name"`
	Message string `json:"message" jsonschema:"Output message"`
}
