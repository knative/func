//go:build unix

package functions

import (
	"errors"
	"syscall"
)

// processAlive reports whether a process with the given PID exists.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false // 0 and negative PIDs address process groups
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// interruptProcess asks the process with the given PID to stop.
func interruptProcess(pid int) error {
	if pid <= 0 {
		return errors.New("invalid pid")
	}
	return syscall.Kill(pid, syscall.SIGINT)
}

// killProcess forcibly stops the process with the given PID.
func killProcess(pid int) error {
	if pid <= 0 {
		return errors.New("invalid pid")
	}
	return syscall.Kill(pid, syscall.SIGKILL)
}
