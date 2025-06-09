package cmd

import (
	"log"

	"github.com/spf13/cobra"
	"knative.dev/func/pkg/mcp"
)

func NewMCPServerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start MCP server",
		Long: `
NAME
	{{rootCmdUse}} mcp - start a Model Context Protocol (MCP) server

SYNOPSIS
	{{rootCmdUse}} mcp [flags]

DESCRIPTION
	Starts a Model Context Protocol (MCP) server over standard input/output (stdio) transport.
	This implementation aims to support tools for deploying and creating serverless functions.
	
	Note: This command is still under development.

EXAMPLES

	o Run an MCP server:
		{{rootCmdUse}} mcp
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPServer(cmd, args)
		},
	}
	return cmd
}

func runMCPServer(cmd *cobra.Command, args []string) error {
	s := mcp.NewServer()
	if err := s.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
		return err
	}
	return nil
}
