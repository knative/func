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
	path, err := resolveRunPath(input.Path)
	if err != nil {
		err = fmt.Errorf("unable to resolve function path: %w", err)
		return
	}

	// reserve claims path before the subprocess is started, so that two
	// concurrent "run" calls for the same path cannot both spawn a
	// process; the loser is rejected here, before any process is started.
	if err = s.runs.reserve(path); err != nil {
		return
	}

	readyCtx, cancel := context.WithTimeout(ctx, runReadyTimeout)
	defer cancel()

	pid, host, port, stop, err := s.starter.Start(readyCtx, "run", input.Args(path)...)
	if err != nil {
		s.runs.release(path)
		err = fmt.Errorf("unable to run function: %w", err)
		return
	}

	s.runs.activate(path, pid, stop)

	output = RunOutput{
		Pid: pid,
		URL: fmt.Sprintf("http://%s:%s", host, port),
	}
	return
}

// RunInput defines the input parameters for the run tool.
type RunInput struct {
	Path     string  `json:"path" jsonschema:"required,Absolute path to the function project directory"`
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
