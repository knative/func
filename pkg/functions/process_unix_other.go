//go:build unix && !linux && !darwin

package functions

import "errors"

// processExecutable is not implemented on this platform.
func processExecutable(int) (string, error) {
	return "", errors.New("not supported on this platform")
}
