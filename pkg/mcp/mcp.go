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
	serverName = "func"
	title      = "func"
	version    = "0.1.0"
)

// NOTE: Invoking prompts in some interfaces (such as Claude Code) when all
// tool parameters are optional parameters requires at least one character of
// input. See issue: https://github.com/anthropics/claude-code/issues/5597

// Server is an MCP Server instance
type Server struct {
	OnInit    func(context.Context) // Invoked when the server is initialized
	prefix    string                // Command prefix ("func" or "kn func")
	readonly  atomic.Bool           // disables deploy and delete when true
	factory   ClientFactory         // constructs functions clients for tool handlers
	service   *Service              // native API integration layer
	executor  executor              // used for help text resources only
	transport mcp.Transport         // Transport to use (defaults to StdioTransport)
	impl      *mcp.Server           // implements the protocol
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

// WithClientFactory sets the factory used to construct functions clients.
// Tool handlers invoke the functions client API directly via the service layer.
func WithClientFactory(factory ClientFactory) Option {
	return func(s *Server) {
		s.factory = factory
		s.service = NewService(factory)
	}
}

// WithExecutor sets a custom executor for help resources and legacy tests.
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
			Name:    serverName,
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
	mcp.AddTool(i, healthCheckTool, s.healthcheckHandler)
	mcp.AddTool(i, createTool, s.createHandler)
	mcp.AddTool(i, buildTool, s.buildHandler)
	mcp.AddTool(i, deployTool, s.deployHandler)
	mcp.AddTool(i, listTool, s.listHandler)
	mcp.AddTool(i, deleteTool, s.deleteHandler)
	mcp.AddTool(i, describeTool, s.describeHandler)
	mcp.AddTool(i, invokeTool, s.invokeHandler)
	mcp.AddTool(i, runTool, s.runHandler)
	mcp.AddTool(i, logsTool, s.logsHandler)
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
	i.AddResource(functionStateResource, s.functionStateHandler)
	i.AddResource(languagesResource, s.languagesHandler)
	i.AddResource(templatesResource, s.templatesHandler)

	i.AddResource(newHelpResource(s, "Help", "help for the command root"))
	i.AddResource(newHelpResource(s, "Create Help", "help for 'create'", "create"))
	i.AddResource(newHelpResource(s, "Build Help", "help for 'build'", "build"))
	i.AddResource(newHelpResource(s, "Deploy Help", "help for 'deploy'", "deploy"))
	i.AddResource(newHelpResource(s, "List Help", "help for 'list'", "list"))
	i.AddResource(newHelpResource(s, "Delete Help", "help for delete", "delete"))
	i.AddResource(newHelpResource(s, "Describe Help", "help for 'describe'", "describe"))
	i.AddResource(newHelpResource(s, "Invoke Help", "help for 'invoke'", "invoke"))
	i.AddResource(newHelpResource(s, "Run Help", "help for 'run'", "run"))
	i.AddResource(newHelpResource(s, "Logs Help", "help for 'logs'", "logs"))

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

func (s *Server) requireService() (*Service, error) {
	if s.service == nil {
		return nil, fmt.Errorf("mcp server not configured with a client factory")
	}
	return s.service, nil
}

// Start the MCP server using the configured transport.
func (s *Server) Start(ctx context.Context) error {
	return s.impl.Run(ctx, s.transport)
}

// defaultExecutor shells out to the func CLI for help resources.
type defaultExecutor struct {
	s *Server
}

func (e defaultExecutor) Execute(ctx context.Context, subcommand string, args ...string) ([]byte, error) {
	cmdParts := buildArgs(e.s.prefix, subcommand, args)
	cmd := exec.CommandContext(ctx, cmdParts[0], cmdParts[1:]...)
	return cmd.CombinedOutput()
}

// buildArgs constructs the ordered argument list for execution.
func buildArgs(prefix, subcommand string, args []string) []string {
	parts := strings.Fields(prefix)
	if subcommand != "" {
		parts = append(parts, subcommand)
	}
	return append(parts, args...)
}
