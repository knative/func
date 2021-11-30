package cmd

import (
	"fmt"
	"os"
	"testing"

	"github.com/ory/viper"
	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/mock"
)

func TestRunRun(t *testing.T) {

	tests := []struct {
		name         string
		fileContents string
		buildErrors  bool
		buildFlag    bool
		shouldBuild  bool
		shouldRun    bool
	}{
		{
			name: "Should attempt to run even if build is skipped",
			fileContents: `name: test-func
runtime: go
created: 2009-11-10 23:00:00`,
			buildFlag:   false,
			shouldBuild: false,
			shouldRun:   true,
		},
		{
			name: "Prebuilt image doesn't get built again",
			fileContents: `name: test-func
runtime: go
image: unexistant
created: 2009-11-10 23:00:00`,
			buildFlag:   true,
			shouldBuild: false,
			shouldRun:   true,
		},
		{
			name: "Build when image is empty and build flag is true",
			fileContents: `name: test-func
runtime: go
created: 2009-11-10 23:00:00`,
			buildFlag:   true,
			shouldBuild: true,
			shouldRun:   true,
		},
		{
			name: "Build error skips execution",
			fileContents: `name: test-func
runtime: go
created: 2009-11-10 23:00:00`,
			buildFlag:   true,
			shouldBuild: true,
			shouldRun:   false,
			buildErrors: true,
		},
	}
	for _, tt := range tests {
		mockRunner := mock.NewRunner()
		mockBuilder := mock.NewBuilder()
		errorBuilder := mock.Builder{
			BuildFn: func(f fn.Function) error { return fmt.Errorf("build failed") },
		}
		cmd := NewRunCmd(func(rc runConfig) *fn.Client {
			buildOption := fn.WithBuilder(mockBuilder)
			if tt.buildErrors {
				buildOption = fn.WithBuilder(&errorBuilder)
			}
			return fn.New(
				fn.WithRunner(mockRunner),
				buildOption,
				fn.WithRegistry("ghcr.com/reg"),
			)
		})
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

		cmd.SetArgs([]string{"--path=" + tempDir})
		viper.SetDefault("build", tt.buildFlag)

		_, err = tempFile.WriteString(tt.fileContents)
		if err != nil {
			t.Fatalf("file content was not written %v", err)
		}
		t.Run(tt.name, func(t *testing.T) {
			err := cmd.Execute()
			if err == nil && tt.buildErrors {
				t.Errorf("Expected error: %v but got %v", tt.buildErrors, err)
			}
			if tt.shouldBuild && !(mockBuilder.BuildInvoked || errorBuilder.BuildInvoked) {
				t.Errorf("Function was expected to build is: %v but build execution was: %v", tt.shouldBuild, mockBuilder.BuildInvoked || errorBuilder.BuildInvoked)
			}
			if mockRunner.RunInvoked != tt.shouldRun {
				t.Errorf("Function was expected to run is: %v but run execution was: %v", tt.shouldRun, mockRunner.RunInvoked)
			}
		})
	}
}
