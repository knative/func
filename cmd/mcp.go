package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"knative.dev/func/pkg/mcp"

	fn "knative.dev/func/pkg/functions"
)

func NewMCPCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Model Context Protocol (MCP) server",
		Long: `
NAME
	{{rootCmdUse}} mcp - Model Context Protocol (MCP) server

SYNOPSIS
	{{rootCmdUse}} mcp [command] [flags]
DESCRIPTION
	The Functions Model Context Protocol (MCP) server can be used to give
	agents the power of Functions.

	Configure your agentic client to use the MCP server with command
	"{{rootCmdUse}} mcp start".  Then get the conversation started with

	"Let's create a Function!".

	By default the MCP server operates in read-only mode, allowing Function
	creation, building, and inspection, but preventing deployment and deletion.
	To enable full write access (deploy and delete operations), set the
	environment variable FUNC_ENABLE_MCP_WRITE=true.

	This is an experimental feature, and using an LLM to create and deploy
	code running on your cluster requires careful supervision. Functions is
	an inherently "mutative" tool, so enabling write mode for your LLM is
	essentially giving (sometimes unpredictable) AI the ability to create,
	modify, deploy, and delete Function instances on your currently connected
	cluster.

	The command "{{rootCmdUse}} mcp start" is meant to be invoked by your MCP
	client (such as Claude Code, Cursor, VS Code, Windsurf, etc.); not run
	directly. Configure your client of choice with a new MCP server which runs
	this command.  For example, in Claude Code this can be accomplished with:
	  claude mcp add func func mcp start
	Instructions for other clients can be found at:
	  https://github.com/knative/func/blob/main/docs/mcp-integration/integration.md

AVAILABLE COMMANDS
	start    Start the MCP server (for use by your agent)

EXAMPLES

	o View this help:
		{{rootCmdUse}} mcp --help
`,
	}

	cmd.AddCommand(NewMCPStartCmd(newClient))

	return cmd
}

func NewMCPStartCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the MCP server",
		Long: `
NAME
	{{rootCmdUse}} mcp start - start the Model Context Protocol (MCP) server

SYNOPSIS
	{{rootCmdUse}} mcp start [flags]

DESCRIPTION
	Starts the Model Context Protocol (MCP) server.

	This command is designed to be invoked by MCP clients directly
	(such as Claude Code, Claude Desktop, Cursor, VS Code, Windsurf, etc.);
	not run directly.

	Please see '{{rootCmdUse}} mcp --help' for more information.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPStart(cmd, args, newClient)
		},
	}
	// no flags at this time; future enhancements may be to allow configuring
	// HTTP Stream vs stdio, single vs multiuser modes, etc.  For now
	// we just use a simple gathering of options in runMCPServer.
	return cmd
}

func runMCPStart(cmd *cobra.Command, args []string, newClient ClientFactory) error {
	// Configure write mode
	writeEnabled := false
	if val := os.Getenv("FUNC_ENABLE_MCP_WRITE"); val != "" {
		parsed, err := strconv.ParseBool(val)
		if err != nil {
			return fmt.Errorf("FUNC_ENABLE_MCP_WRITE shuold be a boolean (true/false, 1/0, etc). Received %q", val)
		}
		writeEnabled = parsed
	}

	// Configure 'func' or 'kn func'?
	rootCmd := cmd.Root()
	cmdPrefix := rootCmd.Use

	// Instantiate
	client, done := newClient(ClientConfig{},
		fn.WithMCPServer(mcp.New(mcp.WithPrefix(cmdPrefix))))
	defer done()

	// Start
	return client.StartMCPServer(cmd.Context(), writeEnabled)

}
