package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// config_volumes_list

var configVolumesListTool = &mcp.Tool{
	Name:        "config_volumes_list",
	Title:       "Config Volumes List",
	Description: "Lists the volume configurations for a function.",
	Annotations: &mcp.ToolAnnotations{
		Title:          "Config Volumes List",
		ReadOnlyHint:   true,
		IdempotentHint: true,
	},
}

type ConfigVolumesListInput struct {
	Path    string `json:"path" jsonschema:"required,Path to the function project directory"`
	Verbose *bool  `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i ConfigVolumesListInput) Args() []string {
	args := []string{"volumes", "--path", i.Path}
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

type ConfigVolumesListOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}

func (s *Server) configVolumesListHandler(ctx context.Context, r *mcp.CallToolRequest, input ConfigVolumesListInput) (result *mcp.CallToolResult, output ConfigVolumesListOutput, err error) {
	out, err := s.executor.Execute(ctx, "config", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = ConfigVolumesListOutput{Message: string(out)}
	return
}

// config_volumes_add

var configVolumesAddTool = &mcp.Tool{
	Name:        "config_volumes_add",
	Title:       "Config Volumes Add",
	Description: "Adds a volume to a function's configuration.",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Config Volumes Add",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(false), // additive only; does not overwrite or delete
		IdempotentHint:  false,      // adding the same volume twice will fail
	},
}

type ConfigVolumesAddInput struct {
	Path      string  `json:"path" jsonschema:"required,Path to the function project directory"`
	Type      *string `json:"type,omitempty" jsonschema:"Volume type: configmap, secret, pvc, or emptydir"`
	MountPath *string `json:"mountPath,omitempty" jsonschema:"Mount path for the volume in the container"`
	Source    *string `json:"source,omitempty" jsonschema:"Name of the ConfigMap, Secret, or PVC to mount"`
	Medium    *string `json:"medium,omitempty" jsonschema:"Storage medium for EmptyDir volume: Memory or empty string"`
	Size      *string `json:"size,omitempty" jsonschema:"Maximum size limit for EmptyDir volume (e.g., 1Gi)"`
	ReadOnly  *bool   `json:"readOnly,omitempty" jsonschema:"Mount volume as read-only (only for PVC)"`
	Verbose   *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i ConfigVolumesAddInput) Args() []string {
	args := []string{"volumes", "add", "--path", i.Path}
	args = appendStringFlag(args, "--type", i.Type)
	args = appendStringFlag(args, "--mount-path", i.MountPath)
	args = appendStringFlag(args, "--source", i.Source)
	args = appendStringFlag(args, "--medium", i.Medium)
	args = appendStringFlag(args, "--size", i.Size)
	args = appendBoolFlag(args, "--read-only", i.ReadOnly)
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

type ConfigVolumesAddOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}

func (s *Server) configVolumesAddHandler(ctx context.Context, r *mcp.CallToolRequest, input ConfigVolumesAddInput) (result *mcp.CallToolResult, output ConfigVolumesAddOutput, err error) {
	out, err := s.executor.Execute(ctx, "config", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = ConfigVolumesAddOutput{Message: string(out)}
	return
}

// config_volumes_remove

var configVolumesRemoveTool = &mcp.Tool{
	Name:        "config_volumes_remove",
	Title:       "Config Volumes Remove",
	Description: "Removes a volume from a function's configuration.",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Config Volumes Remove",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(true), // removes data irreversibly from function config
		IdempotentHint:  false,     // removing a non-existent volume will fail
	},
}

type ConfigVolumesRemoveInput struct {
	Path      string `json:"path" jsonschema:"required,Path to the function project directory"`
	MountPath string `json:"mountPath" jsonschema:"required,Mount path of the volume to remove"`
	Verbose   *bool  `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i ConfigVolumesRemoveInput) Args() []string {
	args := []string{"volumes", "remove", "--path", i.Path, "--mount-path", i.MountPath}
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

type ConfigVolumesRemoveOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}

func (s *Server) configVolumesRemoveHandler(ctx context.Context, r *mcp.CallToolRequest, input ConfigVolumesRemoveInput) (result *mcp.CallToolResult, output ConfigVolumesRemoveOutput, err error) {
	if input.MountPath == "" {
		err = fmt.Errorf("'mountPath' must not be empty")
		return
	}
	out, err := s.executor.Execute(ctx, "config", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = ConfigVolumesRemoveOutput{Message: string(out)}
	return
}
