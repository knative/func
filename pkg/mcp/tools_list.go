package mcp

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

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
		IdempotentHint: true,
	},
}

// listHandler calls pkg/functions.Client.List directly rather than shelling
// out to `func list`. Part of the migration tracked in
// https://github.com/knative/func/issues/3771.
func (s *Server) listHandler(ctx context.Context, r *mcp.CallToolRequest, input ListInput) (result *mcp.CallToolResult, output ListOutput, err error) {
	if s.clientProvider == nil {
		err = fmt.Errorf("list tool requires a configured client provider")
		return
	}
	client := s.clientProvider()
	if client == nil {
		err = fmt.Errorf("list tool: client provider returned nil")
		return
	}

	namespace := ""
	if input.Namespace != nil {
		namespace = *input.Namespace
	}
	if input.AllNamespaces != nil && *input.AllNamespaces {
		namespace = ""
	}

	items, err := client.List(ctx, namespace)
	if err != nil {
		return
	}

	output = ListOutput{
		Functions: items,
		Message:   formatListItems(items, namespace),
	}
	return
}

// ListInput defines the input parameters for the list tool.
type ListInput struct {
	AllNamespaces *bool   `json:"allNamespaces,omitempty" jsonschema:"List functions in all namespaces (overrides namespace parameter)"`
	Namespace     *string `json:"namespace,omitempty" jsonschema:"Kubernetes namespace to list functions in (empty: all namespaces)"`
}

// ListOutput defines the structured output returned by the list tool.
// Functions is the canonical machine-readable result; Message is a human
// summary for fallback display.
type ListOutput struct {
	Functions []fn.ListItem `json:"functions" jsonschema:"Deployed functions"`
	Message   string        `json:"message" jsonschema:"Human-readable summary"`
}

func formatListItems(items []fn.ListItem, namespace string) string {
	if len(items) == 0 {
		if namespace != "" {
			return fmt.Sprintf("no functions found in namespace %q", namespace)
		}
		return "no functions found"
	}
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 8, 2, ' ', 0)
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", "NAME", "NAMESPACE", "RUNTIME", "DEPLOYER", "URL", "READY")
	for _, it := range items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", it.Name, it.Namespace, it.Runtime, it.Deployer, it.URL, it.Ready)
	}
	w.Flush()
	return b.String()
}
