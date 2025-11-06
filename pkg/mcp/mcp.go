package mcp

import (
	"context"
	"errors"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	Name    = "func"
	Title   = "func"
	Version = "0.1.0"
)

// NOTE: Invoking prompts in some interfaces (such as Claude Code) when all
// tool parameters are optional parameters requires at least one character of
// input. See issue: https://github.com/anthropics/claude-code/issues/5597

// Server is an MCP Server instance
type Server struct {
	OnInit    func(context.Context) // Invoked when the server is initialized
	prefix    string                // Command prefix ("func" or "kn func")
	readonly  bool                  // enables deploy and delete
	executor  executor
	transport mcp.Transport // Transport to use (defaults to StdioTransport)
	impl      *mcp.Server   // implements the protocol
}

type executor interface {
	Execute(ctx context.Context, subcommand string, args ...string) ([]byte, error)
}

type Option func(*Server)

// WithPrefix sets the command prefix (e.g., "func" or "kn func")
func WithPrefix(prefix string) Option {
	return func(s *Server) {
		s.prefix = prefix
	}
}

// WithExecutor sets a custom executor for running commands; used in tests.
func WithExecutor(executor executor) Option {
	return func(s *Server) {
		s.executor = executor
	}
}

// WithTransport sets a custom transport for the server; used in tests.
func WithTransport(transport mcp.Transport) Option {
	return func(s *Server) {
		s.transport = transport
	}
}

// WithReadonly sets the server to readonly mode.
func WithReadonly(readonly bool) Option {
	return func(s *Server) {
		s.readonly = readonly
	}
}

// New MCP Server
func New(options ...Option) *Server {
	s := &Server{
		prefix:    "func",
		transport: &mcp.StdioTransport{},
		OnInit:    func(_ context.Context) {},
	}
	s.executor = defaultExecutor{s}
	for _, o := range options {
		o(s)
	}

	i := mcp.NewServer(
		&mcp.Implementation{
			Name:    Name,
			Title:   Title,
			Version: Version},
		&mcp.ServerOptions{
			Instructions:       instructions(s.readonly),
			HasPrompts:         true,
			HasResources:       true,
			HasTools:           true,
			InitializedHandler: func(ctx context.Context, _ *mcp.InitializedRequest) { s.OnInit(ctx) },
		})

	// Tools
	// -----
	// One for each command or command group
	mcp.AddTool(i, healthCheckTool, s.healthcheckHandler)
	mcp.AddTool(i, createTool, s.createHandler)
	mcp.AddTool(i, buildTool, s.buildHandler)
	mcp.AddTool(i, deployTool, s.deployHandler)
	mcp.AddTool(i, listTool, s.listHandler)
	mcp.AddTool(i, deleteTool, s.deleteHandler)
	mcp.AddTool(i, configVolumesTool, s.configVolumesHandler)
	mcp.AddTool(i, configLabelsTool, s.configLabelsHandler)
	mcp.AddTool(i, configEnvsTool, s.configEnvsHandler)

	// Resources
	// ---------
	// Current Function state
	i.AddResource(functionStateResource, s.functionStateHandler)

	// Available languages (output of the languages subcommand)
	i.AddResource(languagesResource, s.languagesHandler)

	// Available templates
	i.AddResource(templatesResource, s.templatesHandler)

	// Help
	// A resource for each command which returns its help
	// eg. "config volumes add" -> "func://help/config/volumes/add")
	i.AddResource(newHelpResource(s, "Help", "help for the command root", ""))
	i.AddResource(newHelpResource(s, "Create Help", "help for 'create'", "create"))
	i.AddResource(newHelpResource(s, "Build Help", "help for 'build'", "build"))
	i.AddResource(newHelpResource(s, "Deploy Help", "help for 'deploy'", "deploy"))
	i.AddResource(newHelpResource(s, "List Help", "help for 'list'", "list"))

	i.AddResource(newHelpResource(s, "Volumes Help", "general help for volumes", "config", "volumes"))
	i.AddResource(newHelpResource(s, "Volumes Add Help", "help for 'config volumes add'", "config", "volumes", "add"))
	i.AddResource(newHelpResource(s, "Volumes Remove Help", "help for 'config volumes remove'", "list", "volumes", "remove"))

	i.AddResource(newHelpResource(s, "Labels Help", "general help for labels", "config", "labels"))
	i.AddResource(newHelpResource(s, "Labels Add Help", "help for 'config labels add'", "config", "labels", "add"))
	i.AddResource(newHelpResource(s, "Labels Remove Help", "help for 'config labels remove'", "list", "labels", "remove"))

	i.AddResource(newHelpResource(s, "Envs Help", "general help for environment variables", "config", "envs"))
	i.AddResource(newHelpResource(s, "Envs Add Help", "help for 'config envs add'", "config", "envs", "add"))
	i.AddResource(newHelpResource(s, "Envs Remove Help", "help for 'config envs remove'", "list", "envs", "remove"))

	s.impl = i

	return s
}

// Start the MCP server using the configured transport
func (s *Server) Start(ctx context.Context, writeEnabled bool) error {
	s.readonly = !writeEnabled
	return s.impl.Run(ctx, s.transport)
}

// For now the executor is a simple run of the command "func" or "kn func"
// etc.  This should be replaced with a direct integration with the functions
// client API.
type defaultExecutor struct {
	s *Server
}

func (e defaultExecutor) Execute(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
	// Parse prefix: "func" or "kn func" -> ["func"] or ["kn", "func"]
	cmdParts := strings.Fields(e.s.prefix)
	cmdParts = append(cmdParts, subcommand)
	cmdParts = append(cmdParts, args...)

	cmd := exec.CommandContext(ctx, cmdParts[0], cmdParts[1:]...)
	// Commands always execute in current working directory
	return cmd.CombinedOutput()
}

var ErrWriteDisabled = errors.New("server is not write enabled")
