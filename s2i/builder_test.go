package s2i_test

import (
	"context"
	"errors"
	"testing"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/s2i"
)

// Test_ErrRuntimeRequired ensures that a request to build without a runtime
// defined for the Function yields an ErrRuntimeRequired
func Test_ErrRuntimeRequired(t *testing.T) {
	b := s2i.NewBuilder(true)
	err := b.Build(context.Background(), fn.Function{})

	if !errors.Is(err, s2i.ErrRuntimeRequired) {
		t.Fatal("expected ErrRuntimeRequired not received")
	}
}

// Test_ErrRuntimeNotSupported ensures that a request to build a function whose
// runtime is not yet supported yields an ErrRuntimeNotSupported
func Test_ErrRuntimeNotSupported(t *testing.T) {
	b := s2i.NewBuilder(true)
	err := b.Build(context.Background(), fn.Function{Runtime: "unsupported"})

	if !errors.Is(err, s2i.ErrRuntimeNotSupported) {
		t.Fatal("expected ErrRuntimeNotSupported not received")
	}
}
