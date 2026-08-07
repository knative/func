package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var runTool = &mcp.Tool{
	Name:        "run",
	Title:       "Run Function Locally",
	Description: "Run a Function locally (building it first if needed) and return its process ID and URL.",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Run Function Locally",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(false),
		IdempotentHint:  false, // calling run again for a path already running is rejected, not idempotent
	},
}

func (s *Server) runHandler(ctx context.Context, r *mcp.CallToolRequest, input RunInput) (result *mcp.CallToolResult, output RunOutput, err error) {
	if s.readonly.Load() {
		err = fmt.Errorf("the server is currently in read-only mode; to enable write operations, set FUNC_ENABLE_MCP_WRITE in the server environment and restart the server")
		return
	}

	path, err := resolveRunPath(input.Path)
	if err != nil {
		err = fmt.Errorf("unable to resolve function path: %w", err)
		return
	}

	// Fail fast without spawning a process if one is already known to be
	// active. s.runs.add below is the authoritative check that also guards
	// against a race between two concurrent "run" calls for the same path.
	if existing, ok := s.runs.get(path); ok {
		err = fmt.Errorf("a function is already running at %q (pid %d); call run_stop first", path, existing.pid)
		return
	}

	readyCtx, cancel := context.WithTimeout(ctx, runReadyTimeout)
	defer cancel()

	pid, host, port, stop, err := s.starter.Start(readyCtx, "run", input.Args(path)...)
	if err != nil {
		err = fmt.Errorf("unable to run function: %w", err)
		return
	}

	if err = s.runs.add(path, pid, stop); err != nil {
		_ = stop()
		return
	}

	output = RunOutput{
		Pid: pid,
		URL: fmt.Sprintf("http://%s:%s", host, port),
	}
	return
}

// RunInput defines the input parameters for the run tool.
type RunInput struct {
	Path     *string `json:"path,omitempty" jsonschema:"Absolute path to the function project directory (default: server's current working directory)"`
	Registry *string `json:"registry,omitempty" jsonschema:"Container registry for the function image"`
	Build    *bool   `json:"build,omitempty" jsonschema:"Force a rebuild before running (default false; a build still happens automatically if the image is missing or out of date)"`
	Port     *int    `json:"port,omitempty" jsonschema:"Host port to bind (default: 8080, or the first available port)"`
}

// Args builds the "func run" argument list for the resolved, absolute path.
func (i RunInput) Args(path string) []string {
	args := []string{"--path", path, "--json"}

	args = appendStringFlag(args, "--registry", i.Registry)

	if i.Build != nil && *i.Build {
		args = append(args, "--build=true")
	}
	if i.Port != nil {
		args = append(args, "--address", fmt.Sprintf("127.0.0.1:%d", *i.Port))
	}

	return args
}

// RunOutput defines the structured output returned by the run tool.
type RunOutput struct {
	Pid int    `json:"pid" jsonschema:"Process ID of the running function"`
	URL string `json:"url" jsonschema:"URL on which the function is listening"`
}
