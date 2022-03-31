package cmd

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

// TODO there should be a better way for darwin/bsd using kqueue
func waitProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find the process: %w", err)
	}
	defer proc.Release()
	for {
		err = proc.Signal(syscall.Signal(0))
		if err != nil {
			break
		}
		time.Sleep(time.Millisecond * 50)
	}
	return nil
}
