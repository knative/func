package common

import (
	"fmt"

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

type MockLoaderSaver struct {
	LoadFn func(path string) (fn.Function, error)
	SaveFn func(f fn.Function) error
}

func (m MockLoaderSaver) Load(path string) (fn.Function, error) {
	return m.LoadFn(path)
}

func (m MockLoaderSaver) Save(f fn.Function) error {
	return m.SaveFn(f)
}
