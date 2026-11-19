//go:build !unix && !windows

package functions

import "errors"

var errProcessUnsupported = errors.New("not supported on this platform")

// processAlive conservatively reports every process as running, so nothing is
// ever stopped as left over on this platform.
func processAlive(int) bool { return true }

func processExecutable(int) (string, error) { return "", errProcessUnsupported }

func interruptProcess(int) error { return errProcessUnsupported }

func killProcess(int) error { return errProcessUnsupported }
