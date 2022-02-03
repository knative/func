package cmd

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
)

func NewCompletionCmd() *cobra.Command {
	return &cobra.Command{
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
				err = cmd.Root().GenBashCompletion(os.Stdout)
			case "zsh":
				err = cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				err = cmd.Root().GenFishCompletion(os.Stdout, true)
			default:
				err = errors.New("unknown shell, only bash, zsh and fish are supported")
			}

			return err
		},
	}

}
