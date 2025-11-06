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
		IdempotentHint:  true, // Deploying the same function configuration multiple times converges to the same desired state.
	},
}

func (s *Server) deployHandler(ctx context.Context, r *mcp.CallToolRequest, input DeployInput) (result *mcp.CallToolResult, output DeployOutput, err error) {
	if s.readonly {
		err = fmt.Errorf("the server is currently in readonly mode.  Please set FUNC_ENABLE_MCP_WRITE and restart the client")
		return
	}
	out, err := s.executor.Execute(ctx, "deploy", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = DeployOutput{
		Message: string(out),
	}
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

func (i DeployInput) Args() []string {
	args := []string{"--path", i.Path}

	args = appendStringFlag(args, "--builder", i.Builder)
	args = appendStringFlag(args, "--registry", i.Registry)
	args = appendStringFlag(args, "--image", i.Image)
	args = appendStringFlag(args, "--namespace", i.Namespace)
	args = appendStringFlag(args, "--git-url", i.GitURL)
	args = appendStringFlag(args, "--git-branch", i.GitBranch)
	args = appendStringFlag(args, "--git-dir", i.GitDir)
	args = appendStringFlag(args, "--builder-image", i.BuilderImage)
	args = appendStringFlag(args, "--domain", i.Domain)
	args = appendStringFlag(args, "--platform", i.Platform)
	args = appendStringFlag(args, "--build", i.Build)
	args = appendStringFlag(args, "--pvc-size", i.PVCSize)
	args = appendStringFlag(args, "--service-account", i.ServiceAccount)
	args = appendStringFlag(args, "--remote-storage-class", i.RemoteStorageClass)

	args = appendBoolFlag(args, "--push", i.Push)
	args = appendBoolFlag(args, "--registry-insecure", i.RegistryInsecure)
	args = appendBoolFlag(args, "--build-timestamp", i.BuildTimestamp)
	args = appendBoolFlag(args, "--remote", i.Remote)
	args = appendBoolFlag(args, "--verbose", i.Verbose)

	return args
}

// DeployOutput defines the structured output returned by the deploy tool.
type DeployOutput struct {
	URL     string `json:"url,omitempty" jsonschema:"The deployed Function URL"`
	Image   string `json:"image,omitempty" jsonschema:"The Function image name"`
	Message string `json:"message" jsonschema:"Output message"`
}
