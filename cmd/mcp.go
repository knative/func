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
	{{rootCmdUse}} mcp - start a MCP server
		
SYNOPSIS
	TBD
		
DESCRIPTION
	TBD
		
EXAMPLES
	TBD
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
	}
	return nil
}
