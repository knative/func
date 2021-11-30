package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/ory/viper"
	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/mock"
)

// TestBuildInvalidRegistry ensures that running build specifying the name of the
// registry explicitly as an argument invokes the registry validation code.
func TestBuildInvalidRegistry(t *testing.T) {
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

	// Create a command with a client constructor fn that instantiates a client
	// with a the mocked builder.
	cmd := NewBuildCmd(func(_ buildConfig) (*fn.Client, error) {
		return fn.New(fn.WithBuilder(builder)), nil
	})

	// Execute the command
	cmd.SetArgs(args)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("Expected error")
	}
}

func Test_runBuild(t *testing.T) {
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
	}
	for _, tt := range tests {
		mockPusher := mock.NewPusher()
		mockBuilder := mock.NewBuilder()
		cmd := NewBuildCmd(func(bc buildConfig) (*fn.Client, error) {
			return fn.New(
				fn.WithBuilder(mockBuilder),
				fn.WithPusher(mockPusher),
			), nil
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
		_, err = tempFile.WriteString(tt.fileContents)
		if err != nil {
			t.Fatalf("file content was not written %v", err)
		}

		cmd.SetArgs([]string{"--path=" + tempDir})
		viper.SetDefault("push", tt.pushFlag)

		t.Run(tt.name, func(t *testing.T) {
			err := cmd.Execute()
			fmt.Printf("ERROR: %+v\n", err)

			if mockBuilder.BuildInvoked != tt.shouldBuild {
				t.Errorf("Build execution expected: %v but was actually %v", tt.shouldBuild, mockBuilder.BuildInvoked)
			}

			if mockPusher.PushInvoked != tt.shouldPush {
				t.Errorf("Push execution expected: %v but was actually %v", tt.shouldPush, mockPusher.PushInvoked)
			}
		})
	}
}
