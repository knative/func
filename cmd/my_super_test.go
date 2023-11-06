package cmd

import (
	"testing"

	fn "knative.dev/func/pkg/functions"
)

func TestDebugger(t *testing.T) {
	// CD into a new tmp directory
	root := fromTempDirectory(t)

	// Create a new default function in Go
	if _, err := fn.New().Init(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	// cmd := NewDeployCmd(NewTestClient(fn.WithRegistry("docker.io/4114gauron3268")))
	cmd := NewBuildCmd(NewTestClient(fn.WithRegistry("docker.io/4114gauron3268")))
	// cmd.SetArgs([]string{"--builder=pack", "--verbose"}) // Or whatever you're testing
	cmd.SetArgs([]string{"--verbose"}) // Or whatever you're testing

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}
