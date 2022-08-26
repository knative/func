package cmd

import (
	"fmt"
	"os"
	"testing"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/mock"
	. "knative.dev/kn-plugin-func/testing"
)

// TestBuild_ImageFlag ensures that the image flag is used when specified.
func TestBuild_ImageFlag(t *testing.T) {
	var (
		args    = []string{"--image", "docker.io/tigerteam/foo"}
		builder = mock.NewBuilder()
	)

	root, cleanup := Mktemp(t)
	defer cleanup()

	if err := fn.New().Create(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	// Create build command that will use a mock builder.
	cmd := NewBuildCmd(NewClientFactory(func() *fn.Client {
		return fn.New(fn.WithBuilder(builder))
	}))

	// Execute the command
	cmd.SetArgs(args)
	err := cmd.Execute()
	if err != nil {
		t.Fatal("Expected error")
	}

	// Now load the function and ensure that the image is set correctly.
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Image != "docker.io/tigerteam/foo" {
		t.Fatalf("Expected image to be 'docker.io/tigerteam/foo', but got '%v'", f.Image)
	}
}

// TestBuild_RegistryOrImageRequired ensures that when no registry or image are
// provided, and the client has not been instantiated with a default registry,
// an ErrRegistryRequired is received.
func TestBuild_RegistryOrImageRequired(t *testing.T) {
	testRegistryOrImageRequired(NewBuildCmd, t)
}

// TestBuild_ImageAndRegistry
func TestBuild_ImageAndRegistry(t *testing.T) {
	testRegistryOrImageRequired(NewBuildCmd, t)
}

// TestBuild_InvalidRegistry ensures that providing an invalid resitry
// fails with the expected error.
func TestBuild_InvalidRegistry(t *testing.T) {
	testInvalidRegistry(NewBuildCmd, t)
}

// TestBuild_RegistryLoads ensures that a function with a defined registry
// will use this when recalculating .Image on build when no --image is
// explicitly provided.
func TestBuild_RegistryLoads(t *testing.T) {
	testRegistryLoads(NewBuildCmd, t)
}

// TestBuild_BuilderPersists ensures that the builder chosen is read from
// the function by default, and is able to be overridden by flags/env vars.
func TestBuild_BuilderPersists(t *testing.T) {
	testBuilderPersists(NewBuildCmd, t)
}

// TestBuild_ValidateBuilder ensures that the validation function correctly
// identifies valid and invalid builder short names.
func TestBuild_BuilderValidated(t *testing.T) {
	testBuilderValidated(NewBuildCmd, t)
}

func TestBuild_runBuild(t *testing.T) {
	tests := []struct {
		name         string
		pushFlag     bool
		fileContents string
		shouldBuild  bool
		shouldPush   bool
		wantErr      bool
	}{
		{
			name:     "push flag triggers push after build",
			pushFlag: true,
			fileContents: `name: test-func
runtime: go
created: 2009-11-10 23:00:00`,
			shouldBuild: true,
			shouldPush:  true,
		},
		{
			name:     "do not push when --push=false",
			pushFlag: false,
			fileContents: `name: test-func
runtime: go
created: 2009-11-10 23:00:00`,
			shouldBuild: true,
			shouldPush:  false,
		},
		{
			name:     "push flag with failing push",
			pushFlag: true,
			fileContents: `name: test-func
runtime: go
created: 2009-11-10 23:00:00`,
			shouldBuild: true,
			shouldPush:  true,
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPusher := mock.NewPusher()
			failPusher := &mock.Pusher{
				PushFn: func(f fn.Function) (string, error) {
					return "", fmt.Errorf("push failed")
				},
			}
			mockBuilder := mock.NewBuilder()
			cmd := NewBuildCmd(NewClientFactory(func() *fn.Client {
				pusher := mockPusher
				if tt.wantErr {
					pusher = failPusher
				}
				return fn.New(
					fn.WithBuilder(mockBuilder),
					fn.WithPusher(pusher),
				)
			}))

			tempDir, err := os.MkdirTemp("", "func-tests")
			if err != nil {
				t.Fatalf("temp dir couldn't be created %v", err)
			}
			t.Log("tempDir created:", tempDir)
			t.Cleanup(func() {
				os.RemoveAll(tempDir)
			})

			fullPath := tempDir + "/func.yaml"
			tempFile, err := os.Create(fullPath)
			if err != nil {
				t.Fatalf("temp file couldn't be created %v", err)
			}
			_, err = tempFile.WriteString(tt.fileContents)
			if err != nil {
				t.Fatalf("file content was not written %v", err)
			}

			cmd.SetArgs([]string{
				"--path=" + tempDir,
				fmt.Sprintf("--push=%t", tt.pushFlag),
				"--registry=docker.io/tigerteam",
			})

			err = cmd.Execute()
			if tt.wantErr != (err != nil) {
				t.Errorf("Wanted error %v but actually got %v", tt.wantErr, err)
			}

			if mockBuilder.BuildInvoked != tt.shouldBuild {
				t.Errorf("Build execution expected: %v but was actually %v", tt.shouldBuild, mockBuilder.BuildInvoked)
			}

			if tt.shouldPush != (mockPusher.PushInvoked || failPusher.PushInvoked) {
				t.Errorf("Push execution expected: %v but was actually mockPusher invoked: %v failPusher invoked %v", tt.shouldPush, mockPusher.PushInvoked, failPusher.PushInvoked)
			}
		})
	}
}
