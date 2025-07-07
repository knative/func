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
			mcp.WithDescription("Creates a knative function in the current directory"),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Name of the function to be created"),
			),
			mcp.WithString("language",
				mcp.Required(),
				mcp.Description("Language/Runtime of the function to be created"),
			),
			mcp.WithString("cwd",
				mcp.Required(),
				mcp.Description("Current working directory of the MCP client"),
			),
		),
		handleCreateTool,
	)

	mcpServer.AddTool(
		mcp.NewTool("deploy",
			mcp.WithDescription("Deploys the function to the cluster"),
			mcp.WithString("registry",
				mcp.Required(),
				mcp.Description("Name of the registry to be used to push the function image"),
			),
			mcp.WithString("cwd",
				mcp.Required(),
				mcp.Description("Full path of the function to be deployed"),
			),
			mcp.WithString("builder",
				mcp.Required(),
				mcp.Description("Builder to be used to build the function image"),
			),
			mcp.WithBoolean("remote",
				mcp.DefaultBool(false),
				mcp.Description("If true, the function will be deployed remotely"),
			),
		),
		handleDeployTool,
	)

	mcpServer.AddTool(
		mcp.NewTool("list",
			mcp.WithDescription("Lists all the functions deployed in the cluster"),
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
				mcp.Description("Builder to be used to build the function image"),
			),
			mcp.WithString("registry",
				mcp.Required(),
				mcp.Description("Name of the registry to be used to push the function image"),
			),
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

	cmd := exec.Command("func", "create", "-l", language, name)
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
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
	remote, err := request.RequireBool("remote")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	var cmd *exec.Cmd
	if remote {
		cmd = exec.Command("func", "deploy", "--registry", registry, "--builder", builder, "--remote")
	} else {
		cmd = exec.Command("func", "deploy", "--registry", registry, "--builder", builder)
	}
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	body := []byte(fmt.Sprintf(`{"result": "%s"}`, out))
	return mcp.NewToolResultText(string(body)), nil
}

func handleListTool(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	cmd := exec.Command("func", "list")
	out, err := cmd.Output()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
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

	cmd := exec.Command("func", "build", "--builder", builder, "--registry", registry)
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
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
	cmd := exec.Command("func", "delete", name)
	out, err := cmd.Output()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	body := []byte(fmt.Sprintf(`{"result": "%s"}`, out))
	return mcp.NewToolResultText(string(body)), nil
}
