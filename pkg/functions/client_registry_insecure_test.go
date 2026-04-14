package functions_test

import (
	"context"
	"testing"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mock"
	. "knative.dev/func/pkg/testing"
)

// TestClient_Build_RegistryInsecureFromClient ensures that when using the client API
// directly (not via CLI), the WithRegistryInsecure option correctly sets the value.
func TestClient_Build_RegistryInsecureFromClient(t *testing.T) {
	root, cleanup := Mktemp(t)
	defer cleanup()

	// Create client with registryInsecure option
	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRegistryInsecure(true),
		fn.WithBuilder(mock.NewBuilder()),
	)

	// Initialize a function without registryInsecure set
	f, err := client.Init(fn.Function{
		Runtime: "go",
		Root:    root,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Build the function
	f, err = client.Build(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}

	// Verify registryInsecure was set by the client
	if !f.RegistryInsecure {
		t.Fatal("registryInsecure should be true from WithRegistryInsecure, but was false")
	}

	// Write and reload to verify persistence
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}

	f2, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	if !f2.RegistryInsecure {
		t.Fatal("registryInsecure should be persisted as true, but was false after reload")
	}
}

// TestClient_Build_RegistryInsecurePreservesExisting ensures that when a function
// already has registryInsecure set, the client respects the existing value.
func TestClient_Build_RegistryInsecurePreservesExisting(t *testing.T) {
	root, cleanup := Mktemp(t)
	defer cleanup()

	// Initialize a function with registryInsecure: true
	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithBuilder(mock.NewBuilder()),
	)

	f, err := client.Init(fn.Function{
		Runtime:          "go",
		Root:             root,
		RegistryInsecure: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Write the function
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}

	// Create a NEW client with registryInsecure=false (should not override)
	client2 := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithBuilder(mock.NewBuilder()),
		fn.WithRegistryInsecure(false),
	)

	// Load the function
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	// Build with the new client
	f, err = client2.Build(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the original value is preserved
	if !f.RegistryInsecure {
		t.Fatal("registryInsecure should be preserved as true, but was false")
	}
}

// TestClient_RunPipeline_RegistryInsecureFromClient ensures registryInsecure
// is set when using RunPipeline with the client API.
func TestClient_RunPipeline_RegistryInsecureFromClient(t *testing.T) {
	root, cleanup := Mktemp(t)
	defer cleanup()

	// Create client with registryInsecure option
	pipeliner := mock.NewPipelinesProvider()
	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRegistryInsecure(true),
		fn.WithPipelinesProvider(pipeliner),
	)

	// Initialize a function with namespace
	f, err := client.Init(fn.Function{
		Runtime:   "go",
		Root:      root,
		Namespace: "test-ns",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Run pipeline
	_, f, err = client.RunPipeline(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}

	// Verify registryInsecure was set
	if !f.RegistryInsecure {
		t.Fatal("registryInsecure should be true from WithRegistryInsecure, but was false")
	}
}

// TestClient_ConfigurePAC_RegistryInsecureFromClient ensures registryInsecure
// is set when using ConfigurePAC with the client API.
func TestClient_ConfigurePAC_RegistryInsecureFromClient(t *testing.T) {
	root, cleanup := Mktemp(t)
	defer cleanup()

	// Create client with registryInsecure option
	pipeliner := mock.NewPipelinesProvider()
	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRegistryInsecure(true),
		fn.WithPipelinesProvider(pipeliner),
	)

	// Initialize a function
	f, err := client.Init(fn.Function{
		Runtime: "go",
		Root:    root,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Configure PAC
	if err = client.ConfigurePAC(context.Background(), f, nil); err != nil {
		t.Fatal(err)
	}

	// Load the function to verify it was written
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	// Verify registryInsecure was set
	if !f.RegistryInsecure {
		t.Fatal("registryInsecure should be true from WithRegistryInsecure, but was false")
	}
}

// TestClient_Build_RegistryInsecureDefaultFalse ensures that when neither
// the client nor function has registryInsecure set, it defaults to false.
func TestClient_Build_RegistryInsecureDefaultFalse(t *testing.T) {
	root, cleanup := Mktemp(t)
	defer cleanup()

	// Create client WITHOUT registryInsecure option
	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithBuilder(mock.NewBuilder()),
	)

	// Initialize a function without registryInsecure
	f, err := client.Init(fn.Function{
		Runtime: "go",
		Root:    root,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Build the function
	f, err = client.Build(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}

	// Verify registryInsecure defaults to false
	if f.RegistryInsecure {
		t.Fatal("registryInsecure should default to false, but was true")
	}

	// Write and reload
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}

	f2, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	// Verify it stays false after reload
	if f2.RegistryInsecure {
		t.Fatal("registryInsecure should remain false after reload, but was true")
	}
}

// TestClient_Build_RegistryInsecureToggle ensures the value can be toggled
// between true and false across multiple builds.
func TestClient_Build_RegistryInsecureToggle(t *testing.T) {
	root, cleanup := Mktemp(t)
	defer cleanup()

	// Initialize function
	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithBuilder(mock.NewBuilder()),
	)

	f, err := client.Init(fn.Function{
		Runtime: "go",
		Root:    root,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Set to true
	f.RegistryInsecure = true
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}

	// Build and verify it stays true
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	f, err = client.Build(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}

	if !f.RegistryInsecure {
		t.Fatal("registryInsecure should be true")
	}

	// Toggle to false
	f.RegistryInsecure = false
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}

	// Build and verify it stays false
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	f, err = client.Build(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}

	if f.RegistryInsecure {
		t.Fatal("registryInsecure should be false after toggle")
	}

	// Toggle back to true
	f.RegistryInsecure = true
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}

	// Build and verify it's true again
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	f, err = client.Build(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}

	if !f.RegistryInsecure {
		t.Fatal("registryInsecure should be true after second toggle")
	}
}
