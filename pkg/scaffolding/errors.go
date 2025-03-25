package scaffolding

import "fmt"

type ScaffoldingError struct {
	Msg string
	Err error
}

func (e ScaffoldingError) Error() string {
	if e.Msg != "" {
		return fmt.Sprintf("scaffolding error. %v. %v", e.Msg, e.Err)
	}
	return fmt.Sprintf("scaffolding error %v", e.Err)
}

func (e ScaffoldingError) Unwrap() error {
	return e.Err
}

var ErrScaffoldingNotFound = ScaffoldingError{"scaffolding not found", nil}
var ErrSignatureNotFound = ScaffoldingError{"supported signature not found", nil}
var ErrFilesysetmRequired = ScaffoldingError{"filesystem required", nil}

type ErrDetectorNotImplemented struct {
	Runtime string
}

func (e ErrDetectorNotImplemented) Error() string {
	return fmt.Sprintf("the %v signature detector is not yet available", e.Runtime)
}

type ErrRuntimeNotRecognized struct {
	Runtime string
}

func (e ErrRuntimeNotRecognized) Error() string {
	return fmt.Sprintf("signature not found.  The runtime %v is not recognized", e.Runtime)
}
