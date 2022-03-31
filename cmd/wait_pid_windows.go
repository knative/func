package cmd

import (
	"fmt"
	"os"
)

func waitProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find the process: %w", err)
	}
	defer proc.Release()

	for {
		ps, err := proc.Wait()
		if err != nil {
			return fmt.Errorf("failed to wait for the process: %w", err)
		}
		if ps.Exited() {
			return nil
		}
	}
}
