package mcp

import (
	"bytes"
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
	readonly  atomic.Bool           // disables deploy, delete, and build when true
	executor  executor
	transport mcp.Transport  // Transport to use (defaults to StdioTransport)
	impl      *mcp.Server    // implements the protocol
	starter   processStarter // starts long-lived "func run" subprocesses
	runs      *runRegistry   // tracks active local runs, keyed by function path
}

type executor interface {
	Execute(ctx context.Context, subcommand string, args ...string) ([]byte, error)
	// ExecuteSplit runs the command and returns stdout and stderr captured
	// into separate buffers. Unlike Execute (which uses CombinedOutput and
	// therefore offers no guarantee about the relative ordering of stdout
	// and stderr bytes - they're copied by two independently-scheduled
	// goroutines), ExecuteSplit gives each stream its own buffer, so callers
	// that need to parse structured output (e.g. JSON) from stdout can do so
	// without risk of stderr content (warnings, etc.) corrupting the parse.
	ExecuteSplit(ctx context.Context, subcommand string, args ...string) (stdout, stderr []byte, err error)
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

// WithProcessStarter sets a custom process starter for the "run" tool; used
// in tests.
func WithProcessStarter(starter processStarter) Option {
	return func(s *Server) {
		s.starter = starter
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
	s.starter = defaultProcessStarter{s}
	s.runs = newRunRegistry()
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
	mcp.AddTool(i, versionTool, s.versionHandler)
	mcp.AddTool(i, createTool, s.createHandler)
	mcp.AddTool(i, buildTool, s.buildHandler)
	mcp.AddTool(i, deployTool, s.deployHandler)
	mcp.AddTool(i, invokeTool, s.invokeHandler)
	mcp.AddTool(i, listTool, s.listHandler)
	mcp.AddTool(i, describeTool, s.describeHandler)
	mcp.AddTool(i, deleteTool, s.deleteHandler)
	mcp.AddTool(i, runTool, s.runHandler)
	mcp.AddTool(i, runStopTool, s.runStopHandler)
	mcp.AddTool(i, configVolumesListTool, s.configVolumesListHandler)
	mcp.AddTool(i, configVolumesAddTool, s.configVolumesAddHandler)
	mcp.AddTool(i, configVolumesRemoveTool, s.configVolumesRemoveHandler)
	mcp.AddTool(i, configLabelsListTool, s.configLabelsListHandler)
	mcp.AddTool(i, configLabelsAddTool, s.configLabelsAddHandler)
	mcp.AddTool(i, configLabelsRemoveTool, s.configLabelsRemoveHandler)
	mcp.AddTool(i, configEnvsListTool, s.configEnvsListHandler)
	mcp.AddTool(i, configEnvsAddTool, s.configEnvsAddHandler)
	mcp.AddTool(i, configEnvsRemoveTool, s.configEnvsRemoveHandler)
	mcp.AddTool(i, configCITool, s.configCIHandler)
	mcp.AddTool(i, repositoryListTool, s.repositoryListHandler)
	mcp.AddTool(i, repositoryAddTool, s.repositoryAddHandler)
	mcp.AddTool(i, repositoryRenameTool, s.repositoryRenameHandler)
	mcp.AddTool(i, repositoryRemoveTool, s.repositoryRemoveHandler)
	mcp.AddTool(i, configGitSetTool, s.configGitSetHandler)
	mcp.AddTool(i, configGitRemoveTool, s.configGitRemoveHandler)

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
	i.AddResource(newHelpResource(s, "Version Help", "help for 'version'", "version"))
	i.AddResource(newHelpResource(s, "Create Help", "help for 'create'", "create"))
	i.AddResource(newHelpResource(s, "Build Help", "help for 'build'", "build"))
	i.AddResource(newHelpResource(s, "Deploy Help", "help for 'deploy'", "deploy"))
	i.AddResource(newHelpResource(s, "Invoke Help", "help for 'invoke'", "invoke"))
	i.AddResource(newHelpResource(s, "List Help", "help for 'list'", "list"))
	i.AddResource(newHelpResource(s, "Describe Help", "help for 'describe'", "describe"))
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

	i.AddResource(newHelpResource(s, "CI Help", "help for 'config ci'", "config", "ci"))

	i.AddResource(newHelpResource(s, "Repository Help", "general help for repository management", "repository"))
	i.AddResource(newHelpResource(s, "Repository Add Help", "help for 'repository add'", "repository", "add"))
	i.AddResource(newHelpResource(s, "Repository Rename Help", "help for 'repository rename'", "repository", "rename"))
	i.AddResource(newHelpResource(s, "Repository Remove Help", "help for 'repository remove'", "repository", "remove"))

	i.AddResource(newHelpResource(s, "Git Help", "general help for Git pipeline config", "config", "git"))
	i.AddResource(newHelpResource(s, "Git Set Help", "help for 'config git set'", "config", "git", "set"))
	i.AddResource(newHelpResource(s, "Git Remove Help", "help for 'config git remove'", "config", "git", "remove"))

	s.impl = i

	return s
}

// Start the MCP server using the configured transport.
// The server's readonly mode is determined at construction time via
// WithReadonly; it cannot be changed after the server is created.
//
// When Run returns, on normal shutdown (client disconnect, or the caller
// canceling ctx in response to SIGINT/SIGTERM), any Function runs left
// active by the "run" tool are stopped so no subprocess (and the port it
// holds) is left behind. A hard kill of the server process itself (e.g. a
// second SIGKILL) bypasses this; the OS reaps the orphaned children instead.
func (s *Server) Start(ctx context.Context) error {
	err := s.impl.Run(ctx, s.transport)
	s.runs.stopAll()
	return err
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

func (e defaultExecutor) ExecuteSplit(ctx context.Context, subcommand string, args ...string) (stdout, stderr []byte, err error) {
	cmdParts := buildArgs(e.s.prefix, subcommand, args)
	cmd := exec.CommandContext(ctx, cmdParts[0], cmdParts[1:]...)
	// cmd.Dir not set - inherits process working directory which is the current working directory
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.Bytes(), errBuf.Bytes(), err
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
