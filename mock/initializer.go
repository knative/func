package mock

import (
	"fmt"
	"strings"
)

type Initializer struct {
	SupportedRuntimes []string
	InitializeInvoked bool
	InitializeFn      func(runtime, template, path string) error
}

func NewInitializer() *Initializer {
	return &Initializer{
		SupportedRuntimes: []string{"go"},
		InitializeFn:      func(string, string, string) error { return nil },
	}
}

func (i *Initializer) Initialize(runtime, template, path string) error {
	i.InitializeInvoked = true
	if !i.supportsRuntime(runtime) {
		return fmt.Errorf("unsupported runtime '%v'", runtime)
	}
	return i.InitializeFn(runtime, template, path)
}

func (i *Initializer) supportsRuntime(runtime string) bool {
	for _, supported := range i.SupportedRuntimes {
		if strings.ToLower(runtime) == supported {
			return true
		}
	}
	return false
}
