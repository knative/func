package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var configVolumesTool = &mcp.Tool{
	Name:        "config_volumes",
	Title:       "Config Volumes",
	Description: "Manages volume configurations for a function. Can add, remove, or list.",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Config Volumes",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(true),
		IdempotentHint:  false, // Adding the same volume twice or removing a non-existent volume will fail.
	},
}

func (s *Server) configVolumesHandler(ctx context.Context, r *mcp.CallToolRequest, input ConfigVolumesInput) (result *mcp.CallToolResult, output ConfigVolumesOutput, err error) {
	out, err := s.executor.Execute(ctx, "config", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = ConfigVolumesOutput{
		Message: string(out),
	}
	return
}

// ConfigVolumesInput defines the input parameters for the config_volumes tool.
type ConfigVolumesInput struct {
	Action    string  `json:"action" jsonschema:"required,Action to perform: add, remove, or list"`
	Path      string  `json:"path" jsonschema:"required,Path to the function project directory"`
	Type      *string `json:"type,omitempty" jsonschema:"Volume type for add action: configmap, secret, pvc, or emptydir"`
	MountPath *string `json:"mountPath,omitempty" jsonschema:"Mount path for the volume in the container"`
	Source    *string `json:"source,omitempty" jsonschema:"Name of the ConfigMap, Secret, or PVC to mount"`
	Medium    *string `json:"medium,omitempty" jsonschema:"Storage medium for EmptyDir volume: Memory or empty string"`
	Size      *string `json:"size,omitempty" jsonschema:"Maximum size limit for EmptyDir volume (e.g., 1Gi)"`
	ReadOnly  *bool   `json:"readOnly,omitempty" jsonschema:"Mount volume as read-only (only for PVC)"`
	Verbose   *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i ConfigVolumesInput) Args() []string {
	args := []string{"volumes"}

	// Allow "list" as an alias for the default action
	if i.Action != "list" {
		args = append(args, i.Action)
	}

	args = append(args, "--path", i.Path)
	args = appendStringFlag(args, "--type", i.Type)
	args = appendStringFlag(args, "--mount-path", i.MountPath)
	args = appendStringFlag(args, "--source", i.Source)
	args = appendStringFlag(args, "--medium", i.Medium)
	args = appendStringFlag(args, "--size", i.Size)
	args = appendBoolFlag(args, "--read-only", i.ReadOnly)
	args = appendBoolFlag(args, "--verbose", i.Verbose)

	return args
}

// ConfigVolumesOutput defines the structured output returned by the config_volumes tool.
type ConfigVolumesOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}
