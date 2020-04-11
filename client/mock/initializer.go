package mock

import (
	"errors"
	"fmt"
	"strings"
)

type Initializer struct {
	SupportedLanguages []string
	InitializeInvoked  bool
	InitializeFn       func(name, language, path string) error
}

func NewInitializer() *Initializer {
	return &Initializer{
		SupportedLanguages: []string{"go"},
		InitializeFn:       func(string, string, string) error { return nil },
	}
}

func (i *Initializer) Initialize(name, language, path string) error {
	fmt.Printf("Validating language supported: %v\n", language)
	i.InitializeInvoked = true
	if !i.supportsLanguage(language) {
		return errors.New(fmt.Sprintf("unsupported language '%v'", language))
	}
	return i.InitializeFn(name, language, path)
}

func (i *Initializer) supportsLanguage(language string) bool {
	for _, supported := range i.SupportedLanguages {
		if strings.ToLower(language) == supported {
			return true
		}
	}
	return false
}
