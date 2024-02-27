package functions

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrEnvironmentNotFound       = errors.New("environment not found")
	ErrFunctionNotFound          = errors.New("function not found")
	ErrMismatchedName            = errors.New("name passed the function source")
	ErrNameRequired              = errors.New("name required")
	ErrNamespaceRequired         = errors.New("namespace required")
	ErrNotBuilt                  = errors.New("not built")
	ErrNotRunning                = errors.New("function not running")
	ErrRepositoriesNotDefined    = errors.New("custom template repositories location not specified")
	ErrRepositoryNotFound        = errors.New("repository not found")
	ErrRootRequired              = errors.New("function root path is required")
	ErrRuntimeNotFound           = errors.New("language runtime not found")
	ErrRuntimeRequired           = errors.New("language runtime required")
	ErrTemplateMissingRepository = errors.New("template name missing repository prefix")
	ErrTemplateNotFound          = errors.New("template not found")
	ErrTemplatesNotFound         = errors.New("templates path (runtimes) not found")
	ErrContextCanceled           = errors.New("the operation was canceled")

	// TODO: change the wording of this error to not be CLI-specific;
	// eg "registry required".  Then catch the error in the CLI and add the
	// cli-specific usage hints there
	ErrRegistryRequired = errors.New("registry required to build function, please set with `--registry` or the FUNC_REGISTRY environment variable")
)

// ErrNotInitialized indicates that a function is uninitialized
type ErrNotInitialized struct {
	Path string
}

func NewErrNotInitialized(path string) error {
	return &ErrNotInitialized{Path: path}
}

func (e ErrNotInitialized) Error() string {
	if e.Path == "" {
		return "function is not initialized"
	}
	return fmt.Sprintf("'%s' does not contain an initialized function", e.Path)
}

// ErrRuntimeNotRecognized indicates a runtime which is not in the set of
// those known.
type ErrRuntimeNotRecognized struct {
	Runtime string
}

func (e ErrRuntimeNotRecognized) Error() string {
	return fmt.Sprintf("the %q runtime is not recognized", e.Runtime)
}

// ErrRunnerNotImplemented indicates the feature is not available for the
// requested runtime.
type ErrRunnerNotImplemented struct {
	Runtime string
}

func (e ErrRunnerNotImplemented) Error() string {
	return fmt.Sprintf("the %q runtime may only be run containerized.", e.Runtime)
}

type ErrRunTimeout struct {
	Timeout time.Duration
}

func (e ErrRunTimeout) Error() string {
	return fmt.Sprintf("timed out waiting for function to be ready for %s", e.Timeout)
}
