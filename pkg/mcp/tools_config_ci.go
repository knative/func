package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// config_ci

var configCITool = &mcp.Tool{
	Name:  "config_ci",
	Title: "Config CI",
	Description: `Generates a GitHub Actions workflow to build, test, and deploy a function on push.

Requires the 'func' binary to be running with the FUNC_ENABLE_CI_CONFIG=true
environment variable set (experimental feature flag). If this is not set, the
underlying command will fail with an "unknown command" error.

Refuses to overwrite an existing workflow file unless 'force' is set.`,
	Annotations: &mcp.ToolAnnotations{
		Title:           "Config CI",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(false), // refuses to overwrite an existing workflow unless force is set
		IdempotentHint:  false,      // re-running without force will fail once the workflow file exists
	},
}

type ConfigCIInput struct {
	Path                         string  `json:"path" jsonschema:"required,Path to the function project directory"`
	Branch                       *string `json:"branch,omitempty" jsonschema:"Git branch to trigger the workflow on push (defaults to the current branch)"`
	WorkflowName                 *string `json:"workflowName,omitempty" jsonschema:"Custom name for the generated workflow"`
	KubeconfigSecretName         *string `json:"kubeconfigSecretName,omitempty" jsonschema:"Name of the GitHub Actions secret containing the kubeconfig, e.g. secret.YOUR_CUSTOM_KUBECONFIG"`
	RegistryLoginUrlVariableName *string `json:"registryLoginUrlVariableName,omitempty" jsonschema:"Name of the GitHub Actions variable containing the registry login URL, e.g. vars.YOUR_REGISTRY_LOGIN_URL"`
	RegistryUserVariableName     *string `json:"registryUserVariableName,omitempty" jsonschema:"Name of the GitHub Actions variable containing the registry username, e.g. vars.YOUR_REGISTRY_USER"`
	RegistryPassSecretName       *string `json:"registryPassSecretName,omitempty" jsonschema:"Name of the GitHub Actions secret containing the registry password, e.g. secret.YOUR_REGISTRY_PASSWORD"`
	RegistryUrlVariableName      *string `json:"registryUrlVariableName,omitempty" jsonschema:"Name of the GitHub Actions variable containing the full registry URL, e.g. vars.YOUR_REGISTRY_URL"`
	RegistryLogin                *bool   `json:"registryLogin,omitempty" jsonschema:"Add a registry login step to the generated workflow"`
	Remote                       *bool   `json:"remote,omitempty" jsonschema:"Build the function on a Tekton-enabled cluster instead of in the workflow runner"`
	SelfHostedRunner             *bool   `json:"selfHostedRunner,omitempty" jsonschema:"Use a 'self-hosted' runner instead of the default 'ubuntu-latest'"`
	TestStep                     *bool   `json:"testStep,omitempty" jsonschema:"Add a language-specific test step (supported: go, node, typescript, python, quarkus)"`
	Force                        *bool   `json:"force,omitempty" jsonschema:"Overwrite an existing GitHub workflow file"`
	Verbose                      *bool   `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i ConfigCIInput) Args() []string {
	args := []string{"ci", "--path", i.Path}
	args = appendStringFlag(args, "--branch", i.Branch)
	args = appendStringFlag(args, "--workflow-name", i.WorkflowName)
	args = appendStringFlag(args, "--kubeconfig-secret-name", i.KubeconfigSecretName)
	args = appendStringFlag(args, "--registry-login-url-variable-name", i.RegistryLoginUrlVariableName)
	args = appendStringFlag(args, "--registry-user-variable-name", i.RegistryUserVariableName)
	args = appendStringFlag(args, "--registry-pass-secret-name", i.RegistryPassSecretName)
	args = appendStringFlag(args, "--registry-url-variable-name", i.RegistryUrlVariableName)
	args = appendBoolFlag(args, "--registry-login", i.RegistryLogin)
	args = appendBoolFlag(args, "--remote", i.Remote)
	args = appendBoolFlag(args, "--self-hosted-runner", i.SelfHostedRunner)
	args = appendBoolFlag(args, "--test-step", i.TestStep)
	args = appendBoolFlag(args, "--force", i.Force)
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

type ConfigCIOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}

func (s *Server) configCIHandler(ctx context.Context, r *mcp.CallToolRequest, input ConfigCIInput) (result *mcp.CallToolResult, output ConfigCIOutput, err error) {
	out, err := s.executor.Execute(ctx, "config", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = ConfigCIOutput{Message: string(out)}
	return
}
