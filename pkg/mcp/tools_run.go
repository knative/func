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
	if err = validatePath(input.Path); err != nil {
		return
	}
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

	proc, err := s.starter.Start(readyCtx, "run", input.Args(path)...)
	if err != nil {
		s.runs.release(path)
		err = fmt.Errorf("unable to run function: %w", err)
		return
	}

	entry := s.runs.activate(path, proc.Pid, proc.Stop, proc.Exited, proc.Output)

	// The process may have died between becoming ready and now. Don't hand
	// back a URL that is already dead. The run is retired synchronously
	// (rather than left to the exit watcher) so the path is immediately
	// reusable, and its output is both kept for run_logs and quoted here:
	// this is exactly the failure that output explains.
	if entry.hasExited() {
		s.runs.retire(path, entry)
		err = fmt.Errorf("unable to run function: process (pid %d) exited right after becoming ready%s", proc.Pid, exitedOutput(entry))
		return
	}

	output = RunOutput{
		Pid: proc.Pid,
		URL: fmt.Sprintf("http://%s:%s", proc.Host, proc.Port),
	}
	return
}

// exitedOutput formats the retained output of an exited run for inclusion in
// an error message, or returns "" if there is none.
func exitedOutput(e *runEntry) string {
	if e.output == nil {
		return ""
	}
	text, _ := e.output()
	if text == "" {
		return ""
	}
	text, _ = truncateTail(text, maxErrorOutputBytes)
	return "\noutput:\n" + text
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
