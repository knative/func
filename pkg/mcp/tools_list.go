package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	fn "knative.dev/func/pkg/functions"
)

var listTool = &mcp.Tool{
	Name:        "list",
	Title:       "List Functions",
	Description: "Lists all deployed functions in the current namespace, specified namespace, or all namespaces.",
	Annotations: &mcp.ToolAnnotations{
		Title:          "List Functions",
		ReadOnlyHint:   true,
		IdempotentHint: true, // Listing functions with the same parameters multiple times returns consistent results at any point in time.
	},
}

// noFunctionsFoundPrefix is the leading text of the human-readable message
// `func list` prints on stdout (see cmd/list.go printNoFunctionsFound) when
// there are zero results. This is printed even under `--output json`
// instead of an empty JSON array, so it must be special-cased below rather
// than treated as a JSON parse failure.
const noFunctionsFoundPrefix = "no functions found"

func (s *Server) listHandler(ctx context.Context, r *mcp.CallToolRequest, input ListInput) (result *mcp.CallToolResult, output ListOutput, err error) {
	// ExecuteSplit (rather than Execute/CombinedOutput) is required here for
	// the same reason as the describe tool: stdout must stay a clean,
	// unmixed JSON payload so it can be parsed regardless of anything the
	// CLI writes to stderr.
	stdout, stderr, err := s.executor.ExecuteSplit(ctx, "list", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\nstdout: %s\nstderr: %s", err, string(stdout), string(stderr))
		return
	}

	items := []fn.ListItem{}
	trimmed := bytes.TrimSpace(stdout)
	if len(trimmed) != 0 && !bytes.HasPrefix(trimmed, []byte(noFunctionsFoundPrefix)) {
		if err = json.Unmarshal(trimmed, &items); err != nil {
			err = fmt.Errorf("failed to parse list output: %w\n%s", err, string(stdout))
			return
		}
	}

	output = ListOutput{
		Items:    items,
		Warnings: strings.TrimSpace(string(stderr)),
	}
	return
}

// ListInput defines the input parameters for the list tool.
// All fields are optional since list can work without any parameters.
type ListInput struct {
	AllNamespaces *bool   `json:"allNamespaces,omitempty" jsonschema:"List functions in all namespaces (overrides namespace parameter)"`
	Namespace     *string `json:"namespace,omitempty" jsonschema:"Kubernetes namespace to list functions in (default: current namespace)"`
	Verbose       *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i ListInput) Args() []string {
	args := []string{}

	args = appendBoolFlag(args, "--all-namespaces", i.AllNamespaces)
	args = appendStringFlag(args, "--namespace", i.Namespace)
	args = appendBoolFlag(args, "--verbose", i.Verbose)

	// The tool's contract is structured JSON, regardless of caller input.
	args = append(args, "--output", "json")
	return args
}

// ListOutput defines the structured output returned by the list tool.
type ListOutput struct {
	Items []fn.ListItem `json:"items" jsonschema:"Deployed Functions matching the query (empty if none found)"`
	// A non-fatal warning on stderr (e.g. a cluster connectivity issue for
	// one of several configured deployers) can accompany an otherwise
	// successful, but partial, list. Surface it so the agent doesn't
	// mistake a short list for the complete picture.
	Warnings string `json:"warnings,omitempty" jsonschema:"Non-fatal warnings emitted while listing Functions"`
}
