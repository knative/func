package cmd

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
)

func init() {
	root.AddCommand(completionCmd)
}

// completionCmd represents the completion command
var completionCmd = &cobra.Command{
	Use:   "completion <bash|zsh|fish>",
	Short: "Generate completion scripts for bash, fish and zsh",
	Long: `To load completion run

For zsh:
source <(func completion zsh)

If you would like to use alias:
alias f=func
compdef _func f

For bash:
source <(func completion bash)

`,
	ValidArgs: []string{"bash", "zsh", "fish"},
	Args:      cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		if len(args) < 1 {
			return errors.New("missing argument")
		}
		switch args[0] {
		case "bash":
			err = root.GenBashCompletion(os.Stdout)
		case "zsh":
			err = root.GenZshCompletion(os.Stdout)
		case "fish":
			err = root.GenFishCompletion(os.Stdout, true)
		default:
			err = errors.New("unknown shell, only bash, zsh and fish are supported")
		}

		return err
	},
}
