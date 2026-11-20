package functions

import (
	"bytes"
	"errors"

	"golang.org/x/sys/unix"
)

// processExecutable returns the path of the executable the process with the
// given PID runs.
func processExecutable(pid int) (string, error) {
	// kern.procargs2: the argument count (int32), then the executable path,
	// NUL-terminated, then the arguments and environment.
	buf, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return "", err
	}
	if len(buf) < 4 {
		return "", errors.New("malformed kern.procargs2")
	}
	path, _, _ := bytes.Cut(buf[4:], []byte{0})
	return string(path), nil
}
