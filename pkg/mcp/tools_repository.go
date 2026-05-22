package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// repository_list

var repositoryListTool = &mcp.Tool{
	Name:        "repository_list",
	Title:       "Repository List",
	Description: "Lists all installed template repositories, including the default embedded repository.",
	Annotations: &mcp.ToolAnnotations{
		Title:          "Repository List",
		ReadOnlyHint:   true,
		IdempotentHint: true,
	},
}

type RepositoryListInput struct {
	Verbose *bool `json:"verbose,omitempty" jsonschema:"Show the URL of each remote repository"`
}

func (i RepositoryListInput) Args() []string {
	args := []string{"list"}
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

type RepositoryListOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}

func (s *Server) repositoryListHandler(ctx context.Context, _ *mcp.CallToolRequest, input RepositoryListInput) (result *mcp.CallToolResult, output RepositoryListOutput, err error) {
	out, err := s.executor.Execute(ctx, "repository", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = RepositoryListOutput{Message: string(out)}
	return
}

// repository_add

var repositoryAddTool = &mcp.Tool{
	Name:        "repository_add",
	Title:       "Repository Add",
	Description: "Adds a named template repository by URL so that its templates become available when creating new functions.",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Repository Add",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(false), // additive only; does not overwrite existing repositories
		IdempotentHint:  false,      // adding the same name twice will fail
	},
}

type RepositoryAddInput struct {
	Name    string `json:"name" jsonschema:"required,Short name to assign to the repository (e.g. 'boson')"`
	URL     string `json:"url" jsonschema:"required,URL of the git repository to add (may include a branch suffix '#branch')"`
	Verbose *bool  `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i RepositoryAddInput) Args() []string {
	args := []string{"add", i.Name, i.URL}
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

type RepositoryAddOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}

func (s *Server) repositoryAddHandler(ctx context.Context, _ *mcp.CallToolRequest, input RepositoryAddInput) (result *mcp.CallToolResult, output RepositoryAddOutput, err error) {
	if s.readonly.Load() {
		err = fmt.Errorf("the server is currently in readonly mode. Please set FUNC_ENABLE_MCP_WRITE and restart the MCP server")
		return
	}
	out, err := s.executor.Execute(ctx, "repository", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = RepositoryAddOutput{Message: string(out)}
	return
}

// repository_rename

var repositoryRenameTool = &mcp.Tool{
	Name:        "repository_rename",
	Title:       "Repository Rename",
	Description: "Renames a previously installed template repository from one name to another.",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Repository Rename",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(false), // mutates metadata only; no data is deleted
		IdempotentHint:  false,
	},
}

type RepositoryRenameInput struct {
	Old     string `json:"old" jsonschema:"required,Current name of the repository to rename"`
	New     string `json:"new" jsonschema:"required,New name for the repository"`
	Verbose *bool  `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i RepositoryRenameInput) Args() []string {
	args := []string{"rename", i.Old, i.New}
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

type RepositoryRenameOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}

func (s *Server) repositoryRenameHandler(ctx context.Context, _ *mcp.CallToolRequest, input RepositoryRenameInput) (result *mcp.CallToolResult, output RepositoryRenameOutput, err error) {
	if s.readonly.Load() {
		err = fmt.Errorf("the server is currently in readonly mode. Please set FUNC_ENABLE_MCP_WRITE and restart the MCP server")
		return
	}
	out, err := s.executor.Execute(ctx, "repository", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = RepositoryRenameOutput{Message: string(out)}
	return
}

// repository_remove

var repositoryRemoveTool = &mcp.Tool{
	Name:        "repository_remove",
	Title:       "Repository Remove",
	Description: "Removes an installed template repository from local disk by name. The default embedded repository cannot be removed.",
	Annotations: &mcp.ToolAnnotations{
		Title:           "Repository Remove",
		ReadOnlyHint:    false,
		DestructiveHint: ptr(true), // removes repository from local disk permanently
		IdempotentHint:  false,     // removing a non-existent repository will fail
	},
}

type RepositoryRemoveInput struct {
	Name    string `json:"name" jsonschema:"required,Name of the installed repository to remove"`
	Verbose *bool  `json:"verbose,omitempty" jsonschema:"Enable verbose logging output"`
}

func (i RepositoryRemoveInput) Args() []string {
	args := []string{"remove", i.Name}
	args = appendBoolFlag(args, "--verbose", i.Verbose)
	return args
}

type RepositoryRemoveOutput struct {
	Message string `json:"message" jsonschema:"Output message"`
}

func (s *Server) repositoryRemoveHandler(ctx context.Context, _ *mcp.CallToolRequest, input RepositoryRemoveInput) (result *mcp.CallToolResult, output RepositoryRemoveOutput, err error) {
	if s.readonly.Load() {
		err = fmt.Errorf("the server is currently in readonly mode. Please set FUNC_ENABLE_MCP_WRITE and restart the MCP server")
		return
	}
	out, err := s.executor.Execute(ctx, "repository", input.Args()...)
	if err != nil {
		err = fmt.Errorf("%w\n%s", err, string(out))
		return
	}
	output = RepositoryRemoveOutput{Message: string(out)}
	return
}
