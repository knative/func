package mock

import (
	"errors"
	"fmt"
	"strings"
)

type Initializer struct {
	SupportedRuntimes []string
	InitializeInvoked bool
	InitializeFn      func(name, runtime, path string) error
}

func NewInitializer() *Initializer {
	return &Initializer{
		SupportedRuntimes: []string{"go"},
		InitializeFn:      func(string, string, string) error { return nil },
	}
}

func (i *Initializer) Initialize(name, runtime, path string) error {
	fmt.Printf("Validating runtime supported: %v\n", runtime)
	i.InitializeInvoked = true
	if !i.supportsRuntime(runtime) {
		return errors.New(fmt.Sprintf("unsupported runtime '%v'", runtime))
	}
	return i.InitializeFn(name, runtime, path)
}

func (i *Initializer) supportsRuntime(runtime string) bool {
	for _, supported := range i.SupportedRuntimes {
		if strings.ToLower(runtime) == supported {
			return true
		}
	}
	return false
}
