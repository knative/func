package oci

import (
	"os"
	"strconv"
)

func processExists(pid string) bool {
	p, err := strconv.Atoi(pid)
	if err != nil {
		return false
	}
	_, err = os.FindProcess(p)
	if err != nil {
		return false
	}
	return true
}
