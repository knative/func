package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// config_git_set

var configGitSetTool = &mcp.Tool{
	Name:  "config_git_set",
	Title: "Config Git Set",
	Description: "Set Git source repository settings for a Function's pipeline-based build and deployment. " +
		"Configures the Git URL, branch, and optional subdirectory, and creates local pipeline templates. " +
		"Optionally configures cluster resources and remote webhook triggers when credentials are supplied.",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Config Git Set",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(false),
		IdempotentHint:  true, // setting the same values repeatedly has the same end state
	},
}

// ConfigGitSetInput defines the input for the config_git_set tool.
//
// git-url and git-branch are required; the CLI will prompt interactively
// for any missing values, which hangs in a non-TTY subprocess.
//
// git-dir defaults to "." (repository root) when not provided, preventing
// the CLI's interactive prompt for that field.
//
// When none of config-local/cluster/remote are set, --config-local is
// forwarded automatically so the CLI skips the webhook confirmation prompt.
type ConfigGitSetInput struct {
	Path          string  `json:"path"                     jsonschema:"required,Absolute path to the Function project directory"`
	GitURL        string  `json:"git_url"                  jsonschema:"required,URL of the Git repository containing the Function source code"`
	GitBranch     string  `json:"git_branch"               jsonschema:"required,Git branch or tag to build from (e.g. main)"`
	GitDir        *string `json:"git_dir,omitempty"        jsonschema:"Subdirectory within the repository where the Function source is located (default: repository root)"`
	GitProvider   *string `json:"git_provider,omitempty"   jsonschema:"Git platform provider for webhook setup; usually auto-detected from the URL (e.g. github, gitlab, gitea)"`
	ConfigLocal   *bool   `json:"config_local,omitempty"   jsonschema:"Create local pipeline template files in the Function directory (default: true when no config flags are set)"`
	ConfigCluster *bool   `json:"config_cluster,omitempty" jsonschema:"Create pipeline credentials and config on the cluster"`
	ConfigRemote  *bool   `json:"config_remote,omitempty"  jsonschema:"Configure a webhook on the remote Git provider (requires gh_access_token for GitHub)"`
	GhAccessToken *string `json:"gh_access_token,omitempty" jsonschema:"GitHub Personal Access Token for webhook creation (scope: public_repo or repo, admin:repo_hook for automatic webhook setup)"`
	Verbose       *bool   `json:"verbose,omitempty"        jsonschema:"Enable verbose logging output"`
}

func (i ConfigGitSetInput) Args() []string {
	args := []string{"git", "set", "--path", i.Path}

	args = append(args, "--git-url", i.GitURL)
	args = append(args, "--git-branch", i.GitBranch)

	// Always pass --git-dir to prevent the interactive prompt.
	// Default to "." (repository root) when not provided or when provided as
	// empty/whitespace-only, as an empty value re-triggers the CLI prompt.
	gitDir := "."
	if i.GitDir != nil && strings.TrimSpace(*i.GitDir) != "" {
		gitDir = *i.GitDir
	}
	args = append(args, "--git-dir", gitDir)

	args = appendStringFlag(args, "--git-provider", i.GitProvider)

	// When no config-* flags are provided, always forward --config-local so
	// the CLI sets HasChanged and skips the interactive webhook-trigger prompt.
	if i.ConfigLocal == nil && i.ConfigCluster == nil && i.ConfigRemote == nil {
		args = append(args, "--config-local")
	} else {
		args = appendBoolFlag(args, "--config-local", i.ConfigLocal)
		args = appendBoolFlag(args, "--config-cluster", i.ConfigCluster)
		args = appendBoolFlag(args, "--config-remote", i.ConfigRemote)
	}

	args = appendStringFlag(args, "--gh-access-token", i.GhAccessToken)
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

type ConfigGitSetOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}

func (s *Server) configGitSetHandler(ctx context.Context, r *mcp.CallToolRequest, input ConfigGitSetInput) (result *mcp.CallToolResult, output ConfigGitSetOutput, err error) {
	if input.Path == "" {
		err = fmt.Errorf("'path' is required")
		return
	}
	if input.GitURL == "" {
		err = fmt.Errorf("'git_url' is required")
		return
	}
	if input.GitBranch == "" {
		err = fmt.Errorf("'git_branch' is required")
		return
	}

	out, err := s.executor.Execute(ctx, "config", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = ConfigGitSetOutput{Message: string(out)}
	return
}

// config_git_remove

var configGitRemoveTool = &mcp.Tool{
	Name:        "config_git_remove",
	Title:       "Config Git Remove",
	Description: "Remove Git source repository settings and associated pipeline resources from a Function. Removes Git config from func.yaml and optionally deletes local pipeline templates and cluster resources.",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Config Git Remove",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(true),
		IdempotentHint:  true,
	},
}

// ConfigGitRemoveInput defines the input for the config_git_remove tool.
//
// When neither delete-local nor delete-cluster is set, --delete-local is
// forwarded automatically to prevent the CLI's interactive "delete all?"
// confirmation prompt in a non-TTY subprocess.
type ConfigGitRemoveInput struct {
	Path          string `json:"path"                    jsonschema:"required,Absolute path to the Function project directory"`
	DeleteLocal   *bool  `json:"delete_local,omitempty"  jsonschema:"Delete local pipeline template files from the Function directory"`
	DeleteCluster *bool  `json:"delete_cluster,omitempty" jsonschema:"Delete pipeline credentials and config from the cluster"`
	Verbose       *bool  `json:"verbose,omitempty"       jsonschema:"Enable verbose logging output"`
}

func (i ConfigGitRemoveInput) Args() []string {
	args := []string{"git", "remove", "--path", i.Path}

	// When no delete-* flags are provided, always forward --delete-local so
	// the CLI sets HasChanged and skips the interactive "delete all?" prompt.
	if i.DeleteLocal == nil && i.DeleteCluster == nil {
		args = append(args, "--delete-local")
	} else {
		args = appendBoolFlag(args, "--delete-local", i.DeleteLocal)
		args = appendBoolFlag(args, "--delete-cluster", i.DeleteCluster)
	}

	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

type ConfigGitRemoveOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}

func (s *Server) configGitRemoveHandler(ctx context.Context, r *mcp.CallToolRequest, input ConfigGitRemoveInput) (result *mcp.CallToolResult, output ConfigGitRemoveOutput, err error) {
	if input.Path == "" {
		err = fmt.Errorf("'path' is required")
		return
	}

	out, err := s.executor.Execute(ctx, "config", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = ConfigGitRemoveOutput{Message: string(out)}
	return
}
