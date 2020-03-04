package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

func init() {
	root.AddCommand(updateCmd)
}

var updateCmd = &cobra.Command{
	Use:        "update",
	Short:      "Update or create a deployed Service Function",
	Long:       `Update deployed function to match the current local state.`,
	SuggestFor: []string{"push", "deploy"},
	RunE:       update,
}

func update(cmd *cobra.Command, args []string) error {
	return errors.New("Update not implemented.")
}
