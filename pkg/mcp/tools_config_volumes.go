package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	fn "knative.dev/func/pkg/functions"
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
	Path string `json:"path" jsonschema:"required,Path to the function project directory"`
}

// VolumeMount is the MCP-facing representation of a configured volume mount.
// It mirrors fn.Volume without the invopop/jsonschema struct tags, which the
// MCP SDK's output-schema generator cannot parse.
type VolumeMount struct {
	Secret                *string         `json:"secret,omitempty" jsonschema:"Name of the Secret to mount"`
	ConfigMap             *string         `json:"configMap,omitempty" jsonschema:"Name of the ConfigMap to mount"`
	PersistentVolumeClaim *VolumePVC      `json:"persistentVolumeClaim,omitempty" jsonschema:"PersistentVolumeClaim mount"`
	EmptyDir              *VolumeEmptyDir `json:"emptyDir,omitempty" jsonschema:"EmptyDir mount"`
	Path                  *string         `json:"path,omitempty" jsonschema:"Mount path in the container"`
}

// VolumePVC mirrors fn.PersistentVolumeClaim for MCP output.
type VolumePVC struct {
	ClaimName *string `json:"claimName,omitempty" jsonschema:"Name of the PersistentVolumeClaim"`
	ReadOnly  bool    `json:"readOnly,omitempty" jsonschema:"Mount the volume read-only"`
}

// VolumeEmptyDir mirrors fn.EmptyDir for MCP output.
type VolumeEmptyDir struct {
	Medium    string  `json:"medium,omitempty" jsonschema:"Storage medium (empty or 'Memory')"`
	SizeLimit *string `json:"sizeLimit,omitempty" jsonschema:"Maximum size limit"`
}

// ConfigVolumesListOutput exposes the configured volume mounts as typed
// structured data; Message is a human-readable summary for fallback display.
type ConfigVolumesListOutput struct {
	Volumes []VolumeMount `json:"volumes" jsonschema:"Configured volume mounts"`
	Message string        `json:"message" jsonschema:"Human-readable summary"`
}

// configVolumesListHandler loads the function at the given path and returns its
// configured volume mounts directly from pkg/functions rather than shelling
// out to `func config volumes`. Part of the migration tracked in
// https://github.com/knative/func/issues/3771.
func (s *Server) configVolumesListHandler(_ context.Context, _ *mcp.CallToolRequest, input ConfigVolumesListInput) (result *mcp.CallToolResult, output ConfigVolumesListOutput, err error) {
	f, err := fn.NewFunction(input.Path)
	if err != nil {
		return
	}
	volumes := make([]VolumeMount, len(f.Run.Volumes))
	for i, v := range f.Run.Volumes {
		vm := VolumeMount{
			Secret:    v.Secret,
			ConfigMap: v.ConfigMap,
			Path:      v.Path,
		}
		if v.PersistentVolumeClaim != nil {
			vm.PersistentVolumeClaim = &VolumePVC{
				ClaimName: v.PersistentVolumeClaim.ClaimName,
				ReadOnly:  v.PersistentVolumeClaim.ReadOnly,
			}
		}
		if v.EmptyDir != nil {
			vm.EmptyDir = &VolumeEmptyDir{
				Medium:    v.EmptyDir.Medium,
				SizeLimit: v.EmptyDir.SizeLimit,
			}
		}
		volumes[i] = vm
	}
	output = ConfigVolumesListOutput{
		Volumes: volumes,
		Message: formatVolumes(f.Run.Volumes),
	}
	return
}

func formatVolumes(volumes []fn.Volume) string {
	if len(volumes) == 0 {
		return "There aren't any configured Volume mounts"
	}
	var b strings.Builder
	b.WriteString("Configured Volumes mounts:\n")
	for _, v := range volumes {
		fmt.Fprintf(&b, " -  %s\n", v.String())
	}
	return b.String()
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
	Path      string  `json:"path" jsonschema:"required,Path to the function project directory"`
	MountPath *string `json:"mountPath,omitempty" jsonschema:"Mount path of the volume to remove"`
	Verbose   *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i ConfigVolumesRemoveInput) Args() []string {
	args := []string{"volumes", "remove", "--path", i.Path}
	args = appendStringFlag(args, "--mount-path", i.MountPath)
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

type ConfigVolumesRemoveOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}

func (s *Server) configVolumesRemoveHandler(ctx context.Context, r *mcp.CallToolRequest, input ConfigVolumesRemoveInput) (result *mcp.CallToolResult, output ConfigVolumesRemoveOutput, err error) {
	out, err := s.executor.Execute(ctx, "config", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = ConfigVolumesRemoveOutput{Message: string(out)}
	return
}
