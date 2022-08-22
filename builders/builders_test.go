package builders_test

import (
	"errors"
	"testing"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/builders"
)

// TestImage_Named ensures that a builder image is returned when
// it exists on the function for a given builder, no defaults.
func TestImage_Named(t *testing.T) {
	f := fn.Function{
		Builder: builders.Pack,
		BuilderImages: map[string]string{
			builders.Pack: "example.com/my/builder-image",
		},
	}

	builderImage, err := builders.Image(f, builders.Pack, make(map[string]string))
	if err != nil {
		t.Fatal(err)
	}
	if builderImage != "example.com/my/builder-image" {
		t.Fatalf("expected 'example.com/my/builder-image', got '%v'", builderImage)
	}
}

// TestImage_ErrRuntimeRequired ensures that the correct error is thrown when
// the function has no builder image yet defined for the named builder, and
// also no runtime to choose from the defaults.
func TestImage_ErrRuntimeRequired(t *testing.T) {
	_, err := builders.Image(fn.Function{}, "", make(map[string]string))
	if err == nil {
		t.Fatalf("did not receive expected error")
	}
	if !errors.Is(err, builders.ErrRuntimeRequired{}) {
		t.Fatalf("error is not an 'ErrRuntimeRequired': '%v'", err)
	}
}

// TestImage_ErrNoDefaultImage ensures that when
func TestImage_ErrNoDefaultImage(t *testing.T) {
	_, err := builders.Image(fn.Function{Runtime: "go"}, "", make(map[string]string))
	if err == nil {
		t.Fatalf("did not receive expected error")
	}
	if !errors.Is(err, builders.ErrNoDefaultImage{Runtime: "go"}) {
		t.Fatalf("did not get 'ErrNoDefaultImage', got '%v'", err)
	}
}

// TestImage_Defaults ensures that, when a default exists in the provided
// map, it is chosen when both runtime is defined on the function and no
// builder image has yet to be defined on the function.
func TestImage_Defaults(t *testing.T) {
	defaults := map[string]string{
		"go": "example.com/go/default-builder-image",
	}
	builderImage, err := builders.Image(fn.Function{Runtime: "go"}, "", defaults)
	if err != nil {
		t.Fatal(err)
	}

	if builderImage != "example.com/go/default-builder-image" {
		t.Fatalf("the default was not chosen")
	}
}

// Test_ErrUnknownBuilder ensures that the error properfly formats.
// This error is used externally by packages which share builders but may
// define their own custom builder, thus actually throwing this error
// is the responsibility of whomever is instantiating builders.
func Test_ErrUnknownBuilder(t *testing.T) {
	var tests = []struct {
		Known    []string
		Expected string
	}{
		{[]string{},
			`"test" is not a known builder`},
		{[]string{"pack"},
			`"test" is not a known builder. The available builder is "pack"`},
		{[]string{"pack", "s2i"},
			`"test" is not a known builder. Available builders are "pack" and "s2i"`},
		{[]string{"pack", "s2i", "custom"},
			`"test" is not a known builder. Available builders are "pack", "s2i" and "custom"`},
	}
	for _, test := range tests {
		e := builders.ErrUnknownBuilder{Name: "test", Known: test.Known}
		if e.Error() != test.Expected {
			t.Fatalf("expected error \"%v\". got \"%v\"", test.Expected, e.Error())
		}
	}
}
