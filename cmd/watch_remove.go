package cmd

import (
	"fmt"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/spf13/cobra"

	"knative.dev/kn-plugin-func/docker"
)

// NewWatchRemoveCmd returns new hidden command that is executed by the run sub-command (see NewRunCmd).
// Users shouldn't invoke this command by themselves.
// This command watches parental `func run` process specified by the `--pid` flag.
// When the parental process exits it removes container specified by the `--ctr` flag.
// This is used to ensure container cleanup even if the `func run` has been closed forcefully (e.g. SIGKILL).
func NewWatchRemoveCmd() *cobra.Command {
	var ctrID string
	var pid int

	var cmd = cobra.Command{
		Use:    "watch-remove",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {

			err := waitProcess(pid)
			if err != nil {
				return fmt.Errorf("failed to wait for the process: %w", err)
			}

			c, _, err := docker.NewClient(client.DefaultDockerHost)
			if err != nil {
				return fmt.Errorf("failed to create docker client: %w", err)
			}

			var timeout = time.Second * 10
			if err = c.ContainerStop(cmd.Context(), ctrID, &timeout); err != nil {
				return fmt.Errorf("failed to stop the container %q: %w", ctrID, err)
			}
			if err = c.ContainerRemove(cmd.Context(), ctrID, types.ContainerRemoveOptions{}); err != nil {
				return fmt.Errorf("failed to stop the container %q: %w", ctrID, err)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&ctrID, "ctr", "c", "", "container id to be removed")
	cmd.Flags().IntVarP(&pid, "pid", "p", -1, "pid to be watched")
	err := cmd.MarkFlagRequired("ctr")
	if err != nil {
		panic(err)
	}
	err = cmd.MarkFlagRequired("pid")
	if err != nil {
		panic(err)
	}

	return &cmd
}
