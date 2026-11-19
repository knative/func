package functions

import (
	"os"
	"strconv"
	"strings"
)

// processExecutable returns the path of the executable the process with the
// given PID runs.
func processExecutable(pid int) (string, error) {
	exe, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(exe, " (deleted)"), nil
}
