package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type MCPServer struct {
	server *server.MCPServer
}

func NewServer() *MCPServer {
	mcpServer := server.NewMCPServer(
		"func-mcp",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	mcpServer.AddTool(
		mcp.NewTool("healthcheck",
			mcp.WithDescription("Checks if the server is running"),
		),
		handleHealthCheckTool,
	)

	mcpServer.AddTool(
		mcp.NewTool("create",
			mcp.WithDescription("Creates a Knative function project in the current or specified directory"),
			mcp.WithString("cwd",
				mcp.Required(),
				mcp.Description("Current working directory of the MCP client"),
			),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Name of the function to be created (used as subdirectory)"),
			),
			mcp.WithString("language",
				mcp.Required(),
				mcp.Description("Language runtime to use (e.g., node, go, python)"),
			),

			// Optional flags
			mcp.WithString("template", mcp.Description("Function template (e.g., http, cloudevents)")),
			mcp.WithString("repository", mcp.Description("URI to Git repo containing the template")),
			mcp.WithBoolean("confirm", mcp.Description("Prompt to confirm options interactively")),
			mcp.WithBoolean("verbose", mcp.Description("Print verbose logs")),
		),
		handleCreateTool,
	)

	mcpServer.AddTool(
		mcp.NewTool("deploy",
			mcp.WithDescription("Deploys the function to the cluster"),
			mcp.WithString("registry",
				mcp.Required(),
				mcp.Description("Registry to be used to push the function image"),
			),
			mcp.WithString("cwd",
				mcp.Required(),
				mcp.Description("Full path of the function to be deployed"),
			),
			mcp.WithString("builder",
				mcp.Required(),
				mcp.Description("Builder to be used to build the function image"),
			),

			// Optional flags
			mcp.WithString("image", mcp.Description("Full image name (overrides registry)")),
			mcp.WithString("namespace", mcp.Description("Namespace to deploy the function into")),
			mcp.WithString("git-url", mcp.Description("Git URL containing the function source")),
			mcp.WithString("git-branch", mcp.Description("Git branch for remote deployment")),
			mcp.WithString("git-dir", mcp.Description("Directory inside the Git repository")),
			mcp.WithString("builder-image", mcp.Description("Custom builder image")),
			mcp.WithString("domain", mcp.Description("Domain for the function route")),
			mcp.WithString("platform", mcp.Description("Target platform to build for (e.g., linux/amd64)")),
			mcp.WithString("path", mcp.Description("Path to the function directory")),
			mcp.WithString("build", mcp.Description(`Build control: "true", "false", or "auto"`)),
			mcp.WithString("pvc-size", mcp.Description("Custom volume size for remote builds")),
			mcp.WithString("service-account", mcp.Description("Kubernetes ServiceAccount to use")),
			mcp.WithString("remote-storage-class", mcp.Description("Storage class for remote volume")),

			mcp.WithBoolean("confirm", mcp.Description("Prompt for confirmation before deploying")),
			mcp.WithBoolean("push", mcp.Description("Push image to registry before deployment")),
			mcp.WithBoolean("verbose", mcp.Description("Print verbose logs")),
			mcp.WithBoolean("registry-insecure", mcp.Description("Skip TLS verification for registry")),
			mcp.WithBoolean("build-timestamp", mcp.Description("Use actual time in image metadata")),
			mcp.WithBoolean("remote", mcp.Description("Trigger remote deployment")),
		),
		handleDeployTool,
	)

	mcpServer.AddTool(
		mcp.NewTool("list",
			mcp.WithDescription("Lists all deployed functions in the current or specified namespace"),

			// Optional flags
			mcp.WithBoolean("all-namespaces", mcp.Description("List functions in all namespaces (overrides --namespace)")),
			mcp.WithString("namespace", mcp.Description("The namespace to list functions in (default is current/active)")),
			mcp.WithString("output", mcp.Description("Output format: human, plain, json, xml, yaml")),
			mcp.WithBoolean("verbose", mcp.Description("Enable verbose output")),
		),
		handleListTool,
	)

	mcpServer.AddTool(
		mcp.NewTool("build",
			mcp.WithDescription("Builds the function image in the current directory"),
			mcp.WithString("cwd",
				mcp.Required(),
				mcp.Description("Current working directory of the MCP client"),
			),
			mcp.WithString("builder",
				mcp.Required(),
				mcp.Description("Builder to be used to build the function image (pack, s2i, host)"),
			),
			mcp.WithString("registry",
				mcp.Required(),
				mcp.Description("Registry to be used to push the function image (e.g. ghcr.io/user)"),
			),

			// Optional flags
			mcp.WithString("builder-image", mcp.Description("Custom builder image to use with buildpacks")),
			mcp.WithString("image", mcp.Description("Full image name (overrides registry + function name)")),
			mcp.WithString("path", mcp.Description("Path to the function directory (default is current dir)")),
			mcp.WithString("platform", mcp.Description("Target platform, e.g. linux/amd64 (for s2i builds)")),

			mcp.WithBoolean("confirm", mcp.Description("Prompt for confirmation before proceeding")),
			mcp.WithBoolean("push", mcp.Description("Push image to registry after building")),
			mcp.WithBoolean("verbose", mcp.Description("Enable verbose logging output")),
			mcp.WithBoolean("registry-insecure", mcp.Description("Skip TLS verification for insecure registries")),
			mcp.WithBoolean("build-timestamp", mcp.Description("Use actual time for image timestamp (buildpacks only)")),
		),
		handleBuildTool,
	)

	mcpServer.AddTool(
		mcp.NewTool("delete",
			mcp.WithDescription("Deletes a function from the cluster"),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Name of the function to be deleted"),
			),

			// Optional flags
			mcp.WithString("namespace", mcp.Description("Namespace to delete from (default: current or active)")),
			mcp.WithString("path", mcp.Description("Path to the function project (default is current directory)")),
			mcp.WithString("all", mcp.Description(`Delete all related resources like Pipelines, Secrets ("true"/"false")`)),

			mcp.WithBoolean("confirm", mcp.Description("Prompt to confirm before deletion")),
			mcp.WithBoolean("verbose", mcp.Description("Enable verbose output")),
		),
		handleDeleteTool,
	)

	mcpServer.AddTool(
		mcp.NewTool("config_volumes",
			mcp.WithDescription("Lists and manages configured volumes for a function"),
			mcp.WithString("action",
				mcp.Required(),
				mcp.Description("The action to perform: 'add' to add a volume, 'remove' to remove a volume, 'list' to list volumes"),
			),
			mcp.WithString("path",
				mcp.Required(),
				mcp.Description("Path to the function. Default is current directory ($FUNC_PATH)"),
			),

			// Optional flags
			mcp.WithString("type", mcp.Description("Volume type: configmap, secret, pvc, or emptydir")),
			mcp.WithString("mount_path", mcp.Description("Mount path for the volume in the function container")),
			mcp.WithString("source", mcp.Description("Name of the ConfigMap, Secret, or PVC to mount (not used for emptydir)")),
			mcp.WithString("medium", mcp.Description("Storage medium for EmptyDir volume: 'Memory' or '' (default)")),
			mcp.WithString("size", mcp.Description("Maximum size limit for EmptyDir volume (e.g., 1Gi)")),
			mcp.WithBoolean("read_only", mcp.Description("Mount volume as read-only (only for PVC)")),
			mcp.WithBoolean("verbose", mcp.Description("Print verbose logs ($FUNC_VERBOSE)")),
		),
		handleConfigVolumesTool,
	)
	mcpServer.AddResource(mcp.NewResource(
		"func://docs",
		"Root Help Command",
		mcp.WithResourceDescription("--help output of the func command"),
		mcp.WithMIMEType("text/plain"),
	), handleRootHelpResource)

	// Static help resources for each command
	mcpServer.AddResource(mcp.NewResource(
		"func://create/docs",
		"Create Command Help",
		mcp.WithResourceDescription("--help output of the 'create' command"),
		mcp.WithMIMEType("text/plain"),
	), func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return runHelpCommand([]string{"create"}, "func://create/docs")
	})

	mcpServer.AddResource(mcp.NewResource(
		"func://build/docs",
		"Build Command Help",
		mcp.WithResourceDescription("--help output of the 'build' command"),
		mcp.WithMIMEType("text/plain"),
	), func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return runHelpCommand([]string{"build"}, "func://build/docs")
	})

	mcpServer.AddResource(mcp.NewResource(
		"func://deploy/docs",
		"Deploy Command Help",
		mcp.WithResourceDescription("--help output of the 'deploy' command"),
		mcp.WithMIMEType("text/plain"),
	), func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return runHelpCommand([]string{"deploy"}, "func://deploy/docs")
	})

	mcpServer.AddResource(mcp.NewResource(
		"func://list/docs",
		"List Command Help",
		mcp.WithResourceDescription("--help output of the 'list' command"),
		mcp.WithMIMEType("text/plain"),
	), func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return runHelpCommand([]string{"list"}, "func://list/docs")
	})

	mcpServer.AddResource(mcp.NewResource(
		"func://delete/docs",
		"Delete Command Help",
		mcp.WithResourceDescription("--help output of the 'delete' command"),
		mcp.WithMIMEType("text/plain"),
	), func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return runHelpCommand([]string{"delete"}, "func://delete/docs")
	})

	mcpServer.AddResource(mcp.NewResource(
		"func://config/volumes/add/docs",
		"Config Volumes Add Command Help",
		mcp.WithResourceDescription("--help output of the 'config volumes add' command"),
		mcp.WithMIMEType("text/plain"),
	), func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return runHelpCommand([]string{"config", "volumes", "add"}, "func://config/volumes/add/docs")
	})

	mcpServer.AddResource(mcp.NewResource(
		"func://config/volumes/remove/docs",
		"Config Volumes Remove Command Help",
		mcp.WithResourceDescription("--help output of the 'config volumes remove' command"),
		mcp.WithMIMEType("text/plain"),
	), func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return runHelpCommand([]string{"config", "volumes", "remove"}, "func://config/volumes/add/docs")
	})

	mcpServer.AddPrompt(mcp.NewPrompt("help",
		mcp.WithPromptDescription("help prompt for the root command"),
	), handleRootHelpPrompt)

	mcpServer.AddPrompt(mcp.NewPrompt("cmd_help",
		mcp.WithPromptDescription("help prompt for a specific command"),
		mcp.WithArgument("cmd",
			mcp.ArgumentDescription("The command for which help is requested"),
			mcp.RequiredArgument(),
		),
	), handleCmdHelpPrompt)

	return &MCPServer{
		server: mcpServer,
	}
}

func (s *MCPServer) Start() error {
	return server.ServeStdio(s.server)
}
