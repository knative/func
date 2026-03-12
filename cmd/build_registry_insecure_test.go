package cmd

import (
	"context"
	"testing"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mock"
	. "knative.dev/func/pkg/testing"
	"knative.dev/pkg/ptr"
)

// TestBuild_RegistryInsecurePersists ensures that the registryInsecure flag
// value is persisted to func.yaml and remembered across consecutive runs.
// See issue https://github.com/knative/func/issues/3489
func TestBuild_RegistryInsecurePersists(t *testing.T) {
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

	// Test 1: Initial state - registryInsecure should be nil
	t.Run("initial_state_is_nil", func(t *testing.T) {
		f, err := fn.NewFunction(root)
		if err != nil {
			t.Fatal(err)
		}

		if f.RegistryInsecure != nil {
			t.Fatalf("initial registryInsecure should be nil, but was %v", *f.RegistryInsecure)
		}
	})

	// Test 2: Set registryInsecure to true with flag
	t.Run("sets_to_true_when_flag_passed", func(t *testing.T) {
		cmd := NewBuildCmd(NewTestClient(
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

		if f.RegistryInsecure == nil {
			t.Fatal("registryInsecure should be true when flag passed, but was nil")
		}
		if !*f.RegistryInsecure {
			t.Fatal("registryInsecure should be true when flag passed, but was false")
		}
	})

	// Test 3: Run build WITHOUT --registry-insecure flag
	// Expected: registryInsecure should remain true (persisted value)
	// This is the key test for issue #3489
	t.Run("persists_true_when_flag_not_passed", func(t *testing.T) {
		cmd := NewBuildCmd(NewTestClient(
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

		if f.RegistryInsecure == nil {
			t.Fatal("registryInsecure should be preserved as true, but was nil")
		}
		if !*f.RegistryInsecure {
			t.Fatal("registryInsecure should be preserved as true, but was false")
		}
	})

	// Test 4: Explicitly set --registry-insecure=false
	// Expected: registryInsecure should be removed (nil)
	t.Run("clears_when_flag_set_to_false", func(t *testing.T) {
		cmd := NewBuildCmd(NewTestClient(
			fn.WithRegistry(TestRegistry),
			fn.WithBuilder(builder),
			fn.WithPusher(pusher),
		))
		cmd.SetArgs([]string{"--registry-insecure=false"})

		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}

		// Load the function and verify registryInsecure is nil
		f, err := fn.NewFunction(root)
		if err != nil {
			t.Fatal(err)
		}

		if f.RegistryInsecure != nil {
			t.Fatalf("registryInsecure should be nil when flag set to false, but was %v", *f.RegistryInsecure)
		}
	})

	// Test 5: Run build again WITHOUT flag after clearing
	// Expected: registryInsecure should stay nil (no pollution)
	t.Run("stays_nil_when_not_set", func(t *testing.T) {
		cmd := NewBuildCmd(NewTestClient(
			fn.WithRegistry(TestRegistry),
			fn.WithBuilder(builder),
			fn.WithPusher(pusher),
		))
		cmd.SetArgs([]string{})

		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}

		// Load the function and verify registryInsecure is still nil
		f, err := fn.NewFunction(root)
		if err != nil {
			t.Fatal(err)
		}

		if f.RegistryInsecure != nil {
			t.Fatalf("registryInsecure should remain nil, but was %v", *f.RegistryInsecure)
		}
	})

	// Test 6: Set back to true and verify multiple consecutive runs
	t.Run("persists_across_multiple_consecutive_runs", func(t *testing.T) {
		// First set it to true
		cmd := NewBuildCmd(NewTestClient(
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
			cmd := NewBuildCmd(NewTestClient(
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

			if f.RegistryInsecure == nil {
				t.Fatalf("build %d: registryInsecure should be true, but was nil", i+1)
			}
			if !*f.RegistryInsecure {
				t.Fatalf("build %d: registryInsecure should be true, but was false", i+1)
			}
		}
	})
}

// TestBuild_RegistryInsecureWithClientAPI ensures that when using the client API
// directly (not via CLI), the WithRegistryInsecure option correctly sets the value.
func TestBuild_RegistryInsecureWithClientAPI(t *testing.T) {
	root := FromTempDirectory(t)

	// Create a function without registryInsecure set
	f := fn.Function{
		Root:     root,
		Name:     "myfunc",
		Runtime:  "go",
		Registry: "example.com/alice",
	}

	var (
		builder = mock.NewBuilder()
		pusher  = mock.NewPusher()
	)

	// Test: Create client with WithRegistryInsecure(true) and verify it's applied
	t.Run("api_sets_registryInsecure_from_client_option", func(t *testing.T) {
		client := fn.New(
			fn.WithRegistry(TestRegistry),
			fn.WithBuilder(builder),
			fn.WithPusher(pusher),
			fn.WithRegistryInsecure(true),
		)

		// Initialize the function
		if _, err := client.Init(f); err != nil {
			t.Fatal(err)
		}

		// Build the function
		f, err := client.Build(context.Background(), f)
		if err != nil {
			t.Fatal(err)
		}

		// Verify registryInsecure was set by the client
		if f.RegistryInsecure == nil {
			t.Fatal("registryInsecure should be set by WithRegistryInsecure, but was nil")
		}
		if !*f.RegistryInsecure {
			t.Fatal("registryInsecure should be true from WithRegistryInsecure, but was false")
		}

		// Write and verify it persists
		if err := f.Write(); err != nil {
			t.Fatal(err)
		}

		// Reload and verify
		f2, err := fn.NewFunction(root)
		if err != nil {
			t.Fatal(err)
		}

		if f2.RegistryInsecure == nil {
			t.Fatal("registryInsecure should be persisted, but was nil")
		}
		if !*f2.RegistryInsecure {
			t.Fatal("registryInsecure should be persisted as true, but was false")
		}
	})

	// Test: Function already has registryInsecure set, client should not override
	t.Run("api_preserves_existing_value", func(t *testing.T) {
		// Set registryInsecure to true
		f.RegistryInsecure = ptr.Bool(true)
		if err := f.Write(); err != nil {
			t.Fatal(err)
		}

		// Create client with registryInsecure=false (should not override)
		client := fn.New(
			fn.WithRegistry(TestRegistry),
			fn.WithBuilder(builder),
			fn.WithPusher(pusher),
			fn.WithRegistryInsecure(false),
		)

		// Load function (has registryInsecure=true)
		f, err := fn.NewFunction(root)
		if err != nil {
			t.Fatal(err)
		}

		// Build - should NOT override existing value
		f, err = client.Build(context.Background(), f)
		if err != nil {
			t.Fatal(err)
		}

		// Verify it still has the original value (true, not overridden to false)
		if f.RegistryInsecure == nil {
			t.Fatal("registryInsecure should be preserved, but was nil")
		}
		if !*f.RegistryInsecure {
			t.Fatal("registryInsecure should be preserved as true, but was false")
		}
	})
}
