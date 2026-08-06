package mcp

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	name    = "func"
	title   = "func"
	version = "0.1.0"
)

// NOTE: Invoking prompts in some interfaces (such as Claude Code) when all
// tool parameters are optional parameters requires at least one character of
// input. See issue: https://github.com/anthropics/claude-code/issues/5597

// Server is an MCP Server instance
type Server struct {
	OnInit    func(context.Context) // Invoked when the server is initialized
	prefix    string                // Command prefix ("func" or "kn func")
	readonly  atomic.Bool           // disables deploy and delete when true
	executor  executor
	transport mcp.Transport // Transport to use (defaults to StdioTransport)
	impl      *mcp.Server   // implements the protocol
}

type executor interface {
	Execute(ctx context.Context, subcommand string, args ...string) ([]byte, error)
}

type Option func(*Server)

// disallowedPrefixChars contains shell metacharacters that must not appear
// in the command prefix to prevent command injection.
const disallowedPrefixChars = "&|;`$(){}[]!><\"'\\\n\r"

// WithPrefix sets the command prefix (e.g., "func" or "kn func").
// The prefix is validated to reject shell metacharacters.
func WithPrefix(prefix string) Option {
	return func(s *Server) {
		if strings.ContainsAny(prefix, disallowedPrefixChars) {
			panic(fmt.Sprintf("mcp: prefix %q contains disallowed shell metacharacters", prefix))
		}
		if strings.TrimSpace(prefix) == "" {
			panic("mcp: prefix must not be empty or whitespace-only")
		}
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
		s.readonly.Store(readonly)
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
			Name:    name,
			Title:   title,
			Version: version},
		&mcp.ServerOptions{
			Instructions:       instructions(s.readonly.Load()),
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
	mcp.AddTool(i, logsTool, s.logsHandler)
	mcp.AddTool(i, deleteTool, s.deleteHandler)
	mcp.AddTool(i, configVolumesListTool, s.configVolumesListHandler)
	mcp.AddTool(i, configVolumesAddTool, s.configVolumesAddHandler)
	mcp.AddTool(i, configVolumesRemoveTool, s.configVolumesRemoveHandler)
	mcp.AddTool(i, configLabelsListTool, s.configLabelsListHandler)
	mcp.AddTool(i, configLabelsAddTool, s.configLabelsAddHandler)
	mcp.AddTool(i, configLabelsRemoveTool, s.configLabelsRemoveHandler)
	mcp.AddTool(i, configEnvsListTool, s.configEnvsListHandler)
	mcp.AddTool(i, configEnvsAddTool, s.configEnvsAddHandler)
	mcp.AddTool(i, configEnvsRemoveTool, s.configEnvsRemoveHandler)

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
	i.AddResource(newHelpResource(s, "Help", "help for the command root"))
	i.AddResource(newHelpResource(s, "Create Help", "help for 'create'", "create"))
	i.AddResource(newHelpResource(s, "Build Help", "help for 'build'", "build"))
	i.AddResource(newHelpResource(s, "Deploy Help", "help for 'deploy'", "deploy"))
	i.AddResource(newHelpResource(s, "List Help", "help for 'list'", "list"))
	i.AddResource(newHelpResource(s, "Logs Help", "help for 'logs'", "logs"))
	i.AddResource(newHelpResource(s, "Delete Help", "help for delete", "delete"))

	i.AddResource(newHelpResource(s, "Volumes Help", "general help for volumes", "config", "volumes"))
	i.AddResource(newHelpResource(s, "Volumes Add Help", "help for 'config volumes add'", "config", "volumes", "add"))
	i.AddResource(newHelpResource(s, "Volumes Remove Help", "help for 'config volumes remove'", "config", "volumes", "remove"))

	i.AddResource(newHelpResource(s, "Labels Help", "general help for labels", "config", "labels"))
	i.AddResource(newHelpResource(s, "Labels Add Help", "help for 'config labels add'", "config", "labels", "add"))
	i.AddResource(newHelpResource(s, "Labels Remove Help", "help for 'config labels remove'", "config", "labels", "remove"))

	i.AddResource(newHelpResource(s, "Envs Help", "general help for environment variables", "config", "envs"))
	i.AddResource(newHelpResource(s, "Envs Add Help", "help for 'config envs add'", "config", "envs", "add"))
	i.AddResource(newHelpResource(s, "Envs Remove Help", "help for 'config envs remove'", "config", "envs", "remove"))

	s.impl = i

	return s
}

// Start the MCP server using the configured transport.
// The server's readonly mode is determined at construction time via
// WithReadonly; it cannot be changed after the server is created.
func (s *Server) Start(ctx context.Context) error {
	return s.impl.Run(ctx, s.transport)
}

// For now the executor is a simple run of the command "func" or "kn func"
// etc.  This should be replaced with a direct integration with the functions
// client API.
type defaultExecutor struct {
	s *Server
}

func (e defaultExecutor) Execute(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
	cmdParts := buildArgs(e.s.prefix, subcommand, args)
	cmd := exec.CommandContext(ctx, cmdParts[0], cmdParts[1:]...)
	// cmd.Dir not set - inherits process working directory which is the current working directory
	return cmd.CombinedOutput()
}

// buildArgs constructs the ordered argument list for execution.
// An empty subcommand is omitted so that commands like "func --help" are
// built correctly rather than "func  --help" with a spurious empty argument.
func buildArgs(prefix, subcommand string, args []string) []string {
	parts := strings.Fields(prefix)
	if subcommand != "" {
		parts = append(parts, subcommand)
	}
	return append(parts, args...)
}
