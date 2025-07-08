package mcp

import (
	"context"
	"fmt"
	"os/exec"

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
		return runHelpCommand("create", "func://create/docs")
	})

	mcpServer.AddResource(mcp.NewResource(
		"func://build/docs",
		"Build Command Help",
		mcp.WithResourceDescription("--help output of the 'build' command"),
		mcp.WithMIMEType("text/plain"),
	), func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return runHelpCommand("build", "func://build/docs")
	})

	mcpServer.AddResource(mcp.NewResource(
		"func://deploy/docs",
		"Deploy Command Help",
		mcp.WithResourceDescription("--help output of the 'deploy' command"),
		mcp.WithMIMEType("text/plain"),
	), func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return runHelpCommand("deploy", "func://deploy/docs")
	})

	mcpServer.AddResource(mcp.NewResource(
		"func://list/docs",
		"List Command Help",
		mcp.WithResourceDescription("--help output of the 'list' command"),
		mcp.WithMIMEType("text/plain"),
	), func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return runHelpCommand("list", "func://list/docs")
	})

	mcpServer.AddResource(mcp.NewResource(
		"func://delete/docs",
		"Delete Command Help",
		mcp.WithResourceDescription("--help output of the 'delete' command"),
		mcp.WithMIMEType("text/plain"),
	), func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return runHelpCommand("delete", "func://delete/docs")
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

func handleRootHelpResource(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	content, err := exec.Command("func", "--help").Output()
	if err != nil {
		return nil, err
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "func://docs",
			MIMEType: "text/plain",
			Text:     string(content),
		},
	}, nil
}

func runHelpCommand(cmd string, uri string) ([]mcp.ResourceContents, error) {
	content, err := exec.Command("func", cmd, "--help").Output()
	if err != nil {
		return nil, err
	}
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      uri,
			MIMEType: "text/plain",
			Text:     string(content),
		},
	}, nil
}

func handleCmdHelpPrompt(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	cmd := request.Params.Arguments["cmd"]
	if cmd == "" {
		return nil, fmt.Errorf("cmd is required")
	}

	return mcp.NewGetPromptResult(
		"Cmd Help Prompt",
		[]mcp.PromptMessage{
			mcp.NewPromptMessage(
				mcp.RoleUser,
				mcp.NewTextContent("What can I do with this func command? Please provide help for the command: "+cmd),
			),
			mcp.NewPromptMessage(
				mcp.RoleAssistant,
				mcp.NewEmbeddedResource(mcp.TextResourceContents{
					URI:      fmt.Sprintf("func://%s/docs", cmd),
					MIMEType: "text/plain",
				}),
			),
		},
	), nil
}

func handleRootHelpPrompt(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return mcp.NewGetPromptResult(
		"Help Prompt",
		[]mcp.PromptMessage{
			mcp.NewPromptMessage(
				mcp.RoleUser,
				mcp.NewTextContent("What can I do with the func command?"),
			),
			mcp.NewPromptMessage(
				mcp.RoleAssistant,
				mcp.NewEmbeddedResource(mcp.TextResourceContents{
					URI:      "func://docs",
					MIMEType: "text/plain",
				}),
			),
		},
	), nil
}

func handleHealthCheckTool(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	body := []byte(fmt.Sprintf(`{"message": "%s"}`, "The MCP server is running!"))
	return mcp.NewToolResultText(string(body)), nil
}

func handleCreateTool(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	cwd, err := request.RequireString("cwd")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	name, err := request.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	language, err := request.RequireString("language")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	args := []string{"create", "-l", language}

	// Optional flags
	if v := request.GetString("template", ""); v != "" {
		args = append(args, "--template", v)
	}
	if v := request.GetString("repository", ""); v != "" {
		args = append(args, "--repository", v)
	}
	if request.GetBool("confirm", false) {
		args = append(args, "--confirm")
	}
	if request.GetBool("verbose", false) {
		args = append(args, "--verbose")
	}

	// `name` is passed as a positional argument (directory to create in)
	args = append(args, name)

	cmd := exec.Command("func", args...)
	cmd.Dir = cwd

	out, err := cmd.CombinedOutput()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("func create failed: %s", out)), nil
	}

	body := []byte(fmt.Sprintf(`{"result": "%s"}`, out))
	return mcp.NewToolResultText(string(body)), nil
}

func handleDeployTool(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	cwd, err := request.RequireString("cwd")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	registry, err := request.RequireString("registry")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	builder, err := request.RequireString("builder")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	args := []string{"deploy", "--builder", builder, "--registry", registry}

	// Optional flags
	if v := request.GetString("image", ""); v != "" {
		args = append(args, "--image", v)
	}
	if v := request.GetString("namespace", ""); v != "" {
		args = append(args, "--namespace", v)
	}
	if v := request.GetString("git-url", ""); v != "" {
		args = append(args, "--git-url", v)
	}
	if v := request.GetString("git-branch", ""); v != "" {
		args = append(args, "--git-branch", v)
	}
	if v := request.GetString("git-dir", ""); v != "" {
		args = append(args, "--git-dir", v)
	}
	if v := request.GetString("builder-image", ""); v != "" {
		args = append(args, "--builder-image", v)
	}
	if v := request.GetString("domain", ""); v != "" {
		args = append(args, "--domain", v)
	}
	if v := request.GetString("platform", ""); v != "" {
		args = append(args, "--platform", v)
	}
	if v := request.GetString("path", ""); v != "" {
		args = append(args, "--path", v)
	}
	if v := request.GetString("build", ""); v != "" {
		args = append(args, "--build", v)
	}
	if v := request.GetString("pvc-size", ""); v != "" {
		args = append(args, "--pvc-size", v)
	}
	if v := request.GetString("service-account", ""); v != "" {
		args = append(args, "--service-account", v)
	}
	if v := request.GetString("remote-storage-class", ""); v != "" {
		args = append(args, "--remote-storage-class", v)
	}

	if request.GetBool("confirm", false) {
		args = append(args, "--confirm")
	}
	if request.GetBool("push", false) {
		args = append(args, "--push")
	}
	if request.GetBool("verbose", false) {
		args = append(args, "--verbose")
	}
	if request.GetBool("registry-insecure", false) {
		args = append(args, "--registry-insecure")
	}
	if request.GetBool("build-timestamp", false) {
		args = append(args, "--build-timestamp")
	}
	if request.GetBool("remote", false) {
		args = append(args, "--remote")
	}

	cmd := exec.Command("func", args...)
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("func deploy failed: %s", out)), nil
	}
	body := []byte(fmt.Sprintf(`{"result": "%s"}`, out))
	return mcp.NewToolResultText(string(body)), nil
}

func handleListTool(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	args := []string{"list"}

	// Optional flags
	if request.GetBool("all-namespaces", false) {
		args = append(args, "--all-namespaces")
	}
	if v := request.GetString("namespace", ""); v != "" {
		args = append(args, "--namespace", v)
	}
	if v := request.GetString("output", ""); v != "" {
		args = append(args, "--output", v)
	}
	if request.GetBool("verbose", false) {
		args = append(args, "--verbose")
	}

	cmd := exec.Command("func", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("func list failed: %s", out)), nil
	}
	body := []byte(fmt.Sprintf(`{"result": "%s"}`, out))
	return mcp.NewToolResultText(string(body)), nil
}

func handleBuildTool(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	cwd, err := request.RequireString("cwd")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	builder, err := request.RequireString("builder")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	registry, err := request.RequireString("registry")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	args := []string{"build", "--builder", builder, "--registry", registry}

	// Optional flags
	if v := request.GetString("builder-image", ""); v != "" {
		args = append(args, "--builder-image", v)
	}
	if v := request.GetString("image", ""); v != "" {
		args = append(args, "--image", v)
	}
	if v := request.GetString("path", ""); v != "" {
		args = append(args, "--path", v)
	}
	if v := request.GetString("platform", ""); v != "" {
		args = append(args, "--platform", v)
	}

	if v := request.GetBool("confirm", false); v {
		args = append(args, "--confirm")
	}
	if v := request.GetBool("push", false); v {
		args = append(args, "--push")
	}
	if v := request.GetBool("verbose", false); v {
		args = append(args, "--verbose")
	}
	if v := request.GetBool("registry-insecure", false); v {
		args = append(args, "--registry-insecure")
	}
	if v := request.GetBool("build-timestamp", false); v {
		args = append(args, "--build-timestamp")
	}

	cmd := exec.Command("func", args...)
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("func build failed: %s", out)), nil
	}
	body := []byte(fmt.Sprintf(`{"result": "%s"}`, out))
	return mcp.NewToolResultText(string(body)), nil
}

func handleDeleteTool(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	name, err := request.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	args := []string{"delete", name}

	// Optional flags
	if v := request.GetString("namespace", ""); v != "" {
		args = append(args, "--namespace", v)
	}
	if v := request.GetString("path", ""); v != "" {
		args = append(args, "--path", v)
	}
	if v := request.GetString("all", ""); v != "" {
		args = append(args, "--all", v)
	}

	if request.GetBool("confirm", false) {
		args = append(args, "--confirm")
	}
	if request.GetBool("verbose", false) {
		args = append(args, "--verbose")
	}

	cmd := exec.Command("func", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("func delete failed: %s", out)), nil
	}

	body := []byte(fmt.Sprintf(`{"result": "%s"}`, out))
	return mcp.NewToolResultText(string(body)), nil
}
