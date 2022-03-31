package cmd

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	sysPidFDOpen = 434
)

func waitProcess(pid int) error {
	fd, _, e := syscall.Syscall(sysPidFDOpen, uintptr(pid), 0, 0)
	if e != 0 {
		return fmt.Errorf("failed to open the pid fd: %w", e)
	}

	pollFDs := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	for {
		n, err := unix.Poll(pollFDs, 1_000)
		if err != nil {
			return fmt.Errorf("failed to poll on the pid fd: %w", err)
		}
		if n == 1 {
			if pollFDs[0].Revents != 0 {
				return nil
			}
		}
	}
}
