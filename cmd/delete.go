package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

func init() {
	root.AddCommand(deleteCmd)
}

var deleteCmd = &cobra.Command{
	Use:        "delete",
	Short:      "Delete deployed Service Function",
	Long:       `Removes the deployed Service Function, but does not delete anything locally.  If no code updates have been made beyond the defaults, this would bring the current codebase back to a state equivalent to having run "create --local".  `,
	SuggestFor: []string{"push", "deploy"},
	RunE:       delete,
}

func delete(cmd *cobra.Command, args []string) error {
	return errors.New("Delete not implemented.")
}
