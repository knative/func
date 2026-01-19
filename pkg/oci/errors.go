package oci

import "fmt"

// BuildErr indicates a general build error occurred.
type BuildErr struct {
	Err error
}

func (e BuildErr) Error() string {
	return fmt.Sprintf("error performing host build. %v", e.Err)
}
