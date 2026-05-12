package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// config_envs_list

var configEnvsListTool = &mcp.Tool{
	Name:        "config_envs_list",
	Title:       "Config Environment Variables List",
	Description: "Lists the environment variables configured for a function.",
	Annotations: &mcp.ToolAnnotations{
		Title:          "Config Environment Variables List",
		ReadOnlyHint:   true,
		IdempotentHint: true,
	},
}

type ConfigEnvsListInput struct {
	Path    string `json:"path" jsonschema:"required,Path to the function project directory"`
	Verbose *bool  `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i ConfigEnvsListInput) Args() []string {
	args := []string{"envs", "--path", i.Path}
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

type ConfigEnvsListOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}

func (s *Server) configEnvsListHandler(ctx context.Context, r *mcp.CallToolRequest, input ConfigEnvsListInput) (result *mcp.CallToolResult, output ConfigEnvsListOutput, err error) {
	out, err := s.executor.Execute(ctx, "config", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = ConfigEnvsListOutput{Message: string(out)}
	return
}

// config_envs_add

var configEnvsAddTool = &mcp.Tool{
	Name:  "config_envs_add",
	Title: "Config Environment Variables Add",
	Description: `Adds an environment variable to a function's configuration.

Supports six source types:
  1. Literal value:      set Name and Value directly.
  2. Local env var:      set Name and Value to "{{ env:LOCAL_VAR }}".
  3. Secret (all keys):  set SecretName only — imports every key from the Secret as env vars.
  4. Secret (one key):   set Name, SecretName, and SecretKey.
  5. ConfigMap (all):    set ConfigMapName only — imports every key from the ConfigMap as env vars.
  6. ConfigMap (one key): set Name, ConfigMapName, and ConfigMapKey.

When SecretName or ConfigMapName is provided, the tool constructs the
appropriate "{{ secret:… }}" or "{{ configMap:… }}" value template automatically.
Value is mutually exclusive with SecretName and ConfigMapName.`,
	Annotations: &mcp.ToolAnnotations{
		Title:           "Config Environment Variables Add",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(false), // additive only; does not overwrite or delete
		IdempotentHint:  false,      // adding the same variable twice will fail
	},
}

type ConfigEnvsAddInput struct {
	Path          string  `json:"path" jsonschema:"required,Path to the function project directory"`
	Name          *string `json:"name,omitempty" jsonschema:"Name of the environment variable"`
	Value         *string `json:"value,omitempty" jsonschema:"Literal value or template expression (e.g. '{{ env:MY_VAR }}') for the environment variable"`
	SecretName    *string `json:"secretName,omitempty" jsonschema:"Name of the Kubernetes Secret to source the value from"`
	SecretKey     *string `json:"secretKey,omitempty" jsonschema:"Key within the Secret; omit to import all keys as env vars"`
	ConfigMapName *string `json:"configMapName,omitempty" jsonschema:"Name of the Kubernetes ConfigMap to source the value from"`
	ConfigMapKey  *string `json:"configMapKey,omitempty" jsonschema:"Key within the ConfigMap; omit to import all keys as env vars"`
	Verbose       *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

// validate returns an error for any illegal input combination before args are
// constructed and forwarded to the CLI. Character-set validation is deferred to
// func's own parser, keeping this layer as a thin pass-through.
func (i ConfigEnvsAddInput) validate() error {
	if i.Value != nil && (i.SecretName != nil || i.ConfigMapName != nil) {
		return fmt.Errorf("value is mutually exclusive with secretName/configMapName; provide one or the other")
	}
	if i.SecretKey != nil && i.SecretName == nil {
		return fmt.Errorf("secretKey requires secretName to be set")
	}
	if i.ConfigMapKey != nil && i.ConfigMapName == nil {
		return fmt.Errorf("configMapKey requires configMapName to be set")
	}
	if i.SecretName != nil && i.ConfigMapName != nil {
		return fmt.Errorf("secretName and configMapName are mutually exclusive; provide only one source")
	}
	// All-keys import (no secretKey/configMapKey) is incompatible with --name: the
	// underlying env validation only allows whole-secret/configMap templates when name is nil.
	if i.SecretName != nil && i.SecretKey == nil && i.Name != nil {
		return fmt.Errorf("name must not be set when importing all keys from a Secret (omit secretKey to import all keys)")
	}
	if i.ConfigMapName != nil && i.ConfigMapKey == nil && i.Name != nil {
		return fmt.Errorf("name must not be set when importing all keys from a ConfigMap (omit configMapKey to import all keys)")
	}
	return nil
}

func (i ConfigEnvsAddInput) Args() []string {
	args := []string{"envs", "add", "--path", i.Path}
	args = appendStringFlag(args, "--name", i.Name)

	if i.Value != nil {
		args = appendStringFlag(args, "--value", i.Value)
	} else if i.SecretName != nil {
		var v string
		if i.SecretKey != nil {
			v = fmt.Sprintf("{{ secret:%s:%s }}", *i.SecretName, *i.SecretKey)
		} else {
			v = fmt.Sprintf("{{ secret:%s }}", *i.SecretName)
		}
		args = append(args, "--value", v)
	} else if i.ConfigMapName != nil {
		var v string
		if i.ConfigMapKey != nil {
			v = fmt.Sprintf("{{ configMap:%s:%s }}", *i.ConfigMapName, *i.ConfigMapKey)
		} else {
			v = fmt.Sprintf("{{ configMap:%s }}", *i.ConfigMapName)
		}
		args = append(args, "--value", v)
	}

	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

type ConfigEnvsAddOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}

func (s *Server) configEnvsAddHandler(ctx context.Context, r *mcp.CallToolRequest, input ConfigEnvsAddInput) (result *mcp.CallToolResult, output ConfigEnvsAddOutput, err error) {
	if s.readonly.Load() {
		err = fmt.Errorf("the server is currently in readonly mode.  Please set FUNC_ENABLE_MCP_WRITE and restart the client")
		return
	}
	if err = input.validate(); err != nil {
		return
	}
	out, err := s.executor.Execute(ctx, "config", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = ConfigEnvsAddOutput{Message: string(out)}
	return
}

// config_envs_remove

var configEnvsRemoveTool = &mcp.Tool{
	Name:        "config_envs_remove",
	Title:       "Config Environment Variables Remove",
	Description: "Removes an environment variable from a function's configuration.",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Config Environment Variables Remove",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(true), // removes data irreversibly from function config
		IdempotentHint:  false,     // removing a non-existent variable will fail
	},
}

type ConfigEnvsRemoveInput struct {
	Path    string  `json:"path" jsonschema:"required,Path to the function project directory"`
	Name    *string `json:"name,omitempty" jsonschema:"Name of the environment variable to remove"`
	Verbose *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i ConfigEnvsRemoveInput) Args() []string {
	args := []string{"envs", "remove", "--path", i.Path}
	args = appendStringFlag(args, "--name", i.Name)
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

type ConfigEnvsRemoveOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}

func (s *Server) configEnvsRemoveHandler(ctx context.Context, r *mcp.CallToolRequest, input ConfigEnvsRemoveInput) (result *mcp.CallToolResult, output ConfigEnvsRemoveOutput, err error) {
	if s.readonly.Load() {
		err = fmt.Errorf("the server is currently in readonly mode.  Please set FUNC_ENABLE_MCP_WRITE and restart the client")
		return
	}
	out, err := s.executor.Execute(ctx, "config", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = ConfigEnvsRemoveOutput{Message: string(out)}
	return
}
