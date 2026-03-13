package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mock"
	. "knative.dev/func/pkg/testing"
)

func TestBuild_RegistryInsecurePersists(t *testing.T) {
	testRegistryInsecurePersists(NewBuildCmd, t)
}

// testRegistryInsecurePersists ensures that the registryInsecure flag
// value is persisted to func.yaml and remembered across consecutive runs.
// See issue https://github.com/knative/func/issues/3489
func testRegistryInsecurePersists(cmdFn func(factory ClientFactory) *cobra.Command, t *testing.T) {
	root := FromTempDirectory(t)

	// Initialize a new function without registryInsecure set
	f := fn.Function{
		Root:     root,
		Name:     "myfunc",
		Runtime:  "go",
		Registry: "example.com/alice",
	}
	if _, err := fn.New().Init(f); err != nil {
		t.Fatal(err)
	}

	var (
		builder = mock.NewBuilder()
		pusher  = mock.NewPusher()
	)

	// Test 1: Initial state - registryInsecure should be false
	t.Run("initial state is false", func(t *testing.T) {
		f, err := fn.NewFunction(root)
		if err != nil {
			t.Fatal(err)
		}

		if f.RegistryInsecure {
			t.Fatal("initial registryInsecure should be false")
		}
	})

	// Test 2: Set registryInsecure to true with flag
	t.Run("sets to true when flag passed", func(t *testing.T) {
		cmd := cmdFn(NewTestClient(
			fn.WithRegistry(TestRegistry),
			fn.WithBuilder(builder),
			fn.WithPusher(pusher),
		))
		cmd.SetArgs([]string{"--registry-insecure=true"})

		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}

		// Load the function and verify registryInsecure is true
		f, err := fn.NewFunction(root)
		if err != nil {
			t.Fatal(err)
		}

		if !f.RegistryInsecure {
			t.Fatal("registryInsecure should be true when flag passed, but was false")
		}
	})

	// Test 3: Run build WITHOUT --registry-insecure flag
	// Expected: registryInsecure should remain true (persisted value)
	// This is the key test for issue #3489
	t.Run("persists true when flag not passed", func(t *testing.T) {
		cmd := cmdFn(NewTestClient(
			fn.WithRegistry(TestRegistry),
			fn.WithBuilder(builder),
			fn.WithPusher(pusher),
		))
		cmd.SetArgs([]string{})

		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}

		// Load the function and verify registryInsecure is still true
		f, err := fn.NewFunction(root)
		if err != nil {
			t.Fatal(err)
		}

		if !f.RegistryInsecure {
			t.Fatal("registryInsecure should be preserved as true, but was false")
		}
	})

	// Test 4: Explicitly set --registry-insecure=false
	// Expected: registryInsecure should be cleared (set to false)
	t.Run("clears when flag set to false", func(t *testing.T) {
		cmd := cmdFn(NewTestClient(
			fn.WithRegistry(TestRegistry),
			fn.WithBuilder(builder),
			fn.WithPusher(pusher),
		))
		cmd.SetArgs([]string{"--registry-insecure=false"})

		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}

		// Load the function and verify registryInsecure is false
		f, err := fn.NewFunction(root)
		if err != nil {
			t.Fatal(err)
		}

		if f.RegistryInsecure {
			t.Fatal("registryInsecure should be false when flag set to false, but was true")
		}
	})

	// Test 5: Run build again WITHOUT flag after clearing
	// Expected: registryInsecure should stay false
	t.Run("stays false when not set", func(t *testing.T) {
		cmd := cmdFn(NewTestClient(
			fn.WithRegistry(TestRegistry),
			fn.WithBuilder(builder),
			fn.WithPusher(pusher),
		))
		cmd.SetArgs([]string{})

		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}

		// Load the function and verify registryInsecure is still false
		f, err := fn.NewFunction(root)
		if err != nil {
			t.Fatal(err)
		}

		if f.RegistryInsecure {
			t.Fatal("registryInsecure should remain false, but was true")
		}
	})

	// Test 6: Set back to true and verify multiple consecutive runs
	t.Run("persists across multiple consecutive runs", func(t *testing.T) {
		// First set it to true
		cmd := cmdFn(NewTestClient(
			fn.WithRegistry(TestRegistry),
			fn.WithBuilder(builder),
			fn.WithPusher(pusher),
		))
		cmd.SetArgs([]string{"--registry-insecure=true"})

		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}

		// Run 3 times without the flag
		for i := 0; i < 3; i++ {
			cmd := cmdFn(NewTestClient(
				fn.WithRegistry(TestRegistry),
				fn.WithBuilder(builder),
				fn.WithPusher(pusher),
			))
			cmd.SetArgs([]string{})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("build %d failed: %v", i+1, err)
			}

			// Load and verify after each build
			f, err := fn.NewFunction(root)
			if err != nil {
				t.Fatalf("loading function after build %d failed: %v", i+1, err)
			}

			if !f.RegistryInsecure {
				t.Fatalf("build %d: registryInsecure should be true, but was false", i+1)
			}
		}
	})
}
