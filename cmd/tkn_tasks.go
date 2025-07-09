package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"knative.dev/func/pkg/pipelines/tekton"
)

func NewTektonClusterTasksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tkn-tasks",
		Short: "List tekton cluster tasks as multi-document yaml",
		Long: `This command prints tekton tekton task embed in the func binary.
Some advanced functionality like OpenShift's Web Console build my require installation of these tasks.
Installation: func tkn-tasks | kubectl apply -f -
`,
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), tekton.GetClusterTasks()+"\n---\n"+tekton.GetDevConsolePipelines())
			return err
		},
	}

	return cmd
}
