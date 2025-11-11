## func mcp

Model Context Protocol (MCP) server

### Synopsis


NAME
	func mcp - Model Context Protocol (MCP) server

SYNOPSIS
	func mcp [command] [flags]
DESCRIPTION
	The Functions Model Context Protocol (MCP) server can be used to give
	agents the power of Functions.

	Configure your agentic client to use the MCP server with command
	"func mcp start".  Then get the conversation started with

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

	The command "func mcp start" is meant to be invoked by your MCP
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
		func mcp --help


### Options

```
  -h, --help   help for mcp
```

### SEE ALSO

* [func](func.md)	 - func manages Knative Functions
* [func mcp start](func_mcp_start.md)	 - Start the MCP server

