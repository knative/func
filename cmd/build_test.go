package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/spf13/cobra"
	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/mock"
)

// TestBuild_InvalidRegistry ensures that running build specifying the name of the
// registry explicitly as an argument invokes the registry validation code.
func TestBuild_InvalidRegistry(t *testing.T) {
	var (
		args    = []string{"--registry", "foo/bar/foobar/boofar"} // provide an invalid registry name
		builder = mock.NewBuilder()                               // with a mock builder
	)

	// Run this test in a temporary directory
	defer fromTempDir(t)()
	// Write a func.yaml config which does not specify an image
	funcYaml := `name: testymctestface
namespace: ""
runtime: go
image: ""
imageDigest: ""
builder: quay.io/boson/faas-go-builder
builders:
  default: quay.io/boson/faas-go-builder
envs: []
annotations: {}
labels: []
created: 2021-01-01T00:00:00+00:00
`
	if err := ioutil.WriteFile("func.yaml", []byte(funcYaml), 0600); err != nil {
		t.Fatal(err)
	}

	// Create build command that will use a mock builder.
	cmd := NewBuildCmd(fn.WithBuilder(builder))

	// Execute the command
	cmd.SetArgs(args)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("Expected error")
	}
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
			// create a build command that uses mock builder and pusher
			// If the test is marked as wanting a failure, use an erroring pusher
			var (
				cmd       *cobra.Command
				builder   = mock.NewBuilder()
				pusher    = mock.NewPusher()
				errPusher = &mock.Pusher{
					PushFn: func(f fn.Function) (string, error) {
						return "", fmt.Errorf("push failed")
					},
				}
			)
			if tt.wantErr {
				cmd = NewBuildCmd(fn.WithBuilder(builder), fn.WithPusher(errPusher))
			} else {
				cmd = NewBuildCmd(fn.WithBuilder(builder), fn.WithPusher(pusher))
			}

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

			if builder.BuildInvoked != tt.shouldBuild {
				t.Errorf("Build execution expected: %v but was actually %v", tt.shouldBuild, builder.BuildInvoked)
			}

			if tt.shouldPush != (pusher.PushInvoked || errPusher.PushInvoked) {
				t.Errorf("Push execution expected: %v but was actually mockPusher invoked: %v failPusher invoked %v", tt.shouldPush, pusher.PushInvoked, errPusher.PushInvoked)
			}
		})
	}
}
