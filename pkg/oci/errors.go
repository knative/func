package oci

import "fmt"

// BuildErr indicates a general build error occurred.
type BuildErr struct {
	Err error
}

func (e BuildErr) Error() string {
	return fmt.Sprintf("error performing host build. %v", e.Err)
}

type ErrBuildInProgress struct {
	Dir string
}

func (e ErrBuildInProgress) Error() string {
	return fmt.Sprintf("a build for this function is associated with an active PID appears to be already in progress %v", e.Dir)
}
