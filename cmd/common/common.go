package common

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	fn "knative.dev/func/pkg/functions"
)

// DefaultLoaderSaver implements FunctionLoaderSaver composite interface
var DefaultLoaderSaver FunctionLoaderSaver = standardLoaderSaver{}

// FunctionLoader loads a function from a filesystem path.
type FunctionLoader interface {
	Load(path string) (fn.Function, error)
}

// FunctionSaver persists a function to storage.
type FunctionSaver interface {
	Save(f fn.Function) error
}

// FunctionLoaderSaver combines loading and saving capabilities for functions.
type FunctionLoaderSaver interface {
	FunctionLoader
	FunctionSaver
}

type standardLoaderSaver struct{}

// Load creates and validates a function from the given filesystem path.
func (s standardLoaderSaver) Load(path string) (fn.Function, error) {
	f, err := fn.NewFunction(path)
	if err != nil {
		return fn.Function{}, fmt.Errorf("failed to create new function (path: %q): %w", path, err)
	}

	if !f.Initialized() {
		return fn.Function{}, fn.NewErrNotInitialized(f.Root)
	}

	return f, nil
}

// Save writes the function configuration to disk.
func (s standardLoaderSaver) Save(f fn.Function) error {
	return f.Write()
}

// NewMockLoaderSaver creates a MockLoaderSaver with default no-op
// implementations.
func NewMockLoaderSaver() *MockLoaderSaver {
	return &MockLoaderSaver{
		LoadFn: func(path string) (fn.Function, error) {
			return fn.Function{}, nil
		},
		SaveFn: func(f fn.Function) error {
			return nil
		},
	}
}

// MockLoaderSaver provides configurable function loading and saving for testing
// purposes.
type MockLoaderSaver struct {
	LoadFn func(path string) (fn.Function, error)
	SaveFn func(f fn.Function) error
}

// Load invokes the configured LoadFn to load a function from the given path.
func (m MockLoaderSaver) Load(path string) (fn.Function, error) {
	return m.LoadFn(path)
}

// Save invokes the configured SaveFn to persist the given function.
func (m MockLoaderSaver) Save(f fn.Function) error {
	return m.SaveFn(f)
}

// CurrentBranchFunc is a function type that retrieves the current git branch for a given path.
type CurrentBranchFunc func(path string) (string, error)

// DefaultCurrentBranch is the default implementation for getting the current git branch.
var DefaultCurrentBranch CurrentBranchFunc = NewGitCliWrapper().CurrentBranch

type gitCliWrapper struct {
	gitCmd string
}

// NewGitCliWrapper creates a new git CLI wrapper using FUNC_GIT env var or "git" as default.
func NewGitCliWrapper() *gitCliWrapper {
	gitCmd := os.Getenv("FUNC_GIT")
	if gitCmd == "" {
		gitCmd = "git"
	}

	return &gitCliWrapper{gitCmd}
}

// CurrentBranch returns the current git branch name for the repository at the given path.
func (g *gitCliWrapper) CurrentBranch(path string) (string, error) {
	if _, err := os.Stat(path); err != nil {
		return "", err
	}

	branch, err := g.execGitCmdWith("-C", path, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return "", fmt.Errorf("could not detect git branch for '%s'. "+
			"Has git been initialized for this Function? %w", path, err)
	}

	return branch, nil
}

// Init initializes a new git repository at the given path with the specified branch.
func (g *gitCliWrapper) Init(path, branch string) (string, error) {
	if _, err := os.Stat(path); err != nil {
		return "", err
	}

	if branch == "" {
		return "", fmt.Errorf("branch cannot be empty")
	}

	return g.execGitCmdWith("init", "-b", branch, path)
}

func (g *gitCliWrapper) execGitCmdWith(args ...string) (string, error) {
	result, err := exec.Command(g.gitCmd, args...).Output()
	if err == nil {
		return strings.TrimSpace(string(result)), nil
	}

	var exitErr *exec.ExitError
	argsJoined := strings.Join(args, " ")
	if errors.As(err, &exitErr) {
		return "", fmt.Errorf("git %s failed: %w\nstderr: %s", argsJoined, err, string(exitErr.Stderr))
	}

	return "", fmt.Errorf("git %s failed: %w", argsJoined, err)
}

// CurrentBranchStub creates a stub CurrentBranchFunc that returns the provided output or error.
var CurrentBranchStub = func(output string, err error) CurrentBranchFunc {
	return func(_ string) (string, error) {
		if err != nil {
			return "", err
		}

		return output, nil
	}
}

// WorkDirFunc is a function type that retrieves the current working directory.
type WorkDirFunc func() (string, error)

// DefaultWorkDir is the default implementation for getting the current working directory.
var DefaultWorkDir WorkDirFunc = os.Getwd

// WorkDirStub creates a stub WorkDirFunc that returns the provided directory or error.
var WorkDirStub = func(dir string, err error) WorkDirFunc {
	return func() (string, error) { return dir, err }
}
