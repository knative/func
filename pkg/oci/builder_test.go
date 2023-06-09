package oci

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	fn "knative.dev/func/pkg/functions"
	. "knative.dev/func/pkg/testing"
)

var TestPlatforms = []fn.Platform{{OS: "linux", Architecture: runtime.GOARCH}}

// TestBuilder_Build ensures that, when given a Go Function, an OCI-compliant
// directory structure is created on .Build in the expected path.
func TestBuilder_Build(t *testing.T) {
	root, done := Mktemp(t)
	defer done()

	client := fn.New(fn.WithVerbose(true))

	f, err := client.Init(fn.Function{Root: root, Runtime: "go"})
	if err != nil {
		t.Fatal(err)
	}

	builder := NewBuilder("", true)

	if err := builder.Build(context.Background(), f, TestPlatforms); err != nil {
		t.Fatal(err)
	}

	last := path(f.Root, fn.RunDataDir, "builds", "last", "oci")

	validateOCI(last, t)
}

// TestBuilder_Concurrency
func TestBuilder_Concurrency(t *testing.T) {
	root, done := Mktemp(t)
	defer done()

	client := fn.New()

	// Initialize a new Go Function
	f, err := client.Init(fn.Function{Root: root, Runtime: "go"})
	if err != nil {
		t.Fatal(err)
	}

	// Concurrency
	//
	// The first builder is setup to use a mock implementation of the
	// builder function which will block until released after first notifying
	// that it has been paused.
	//
	// When the test receives the message that the builder has been paused, it
	// starts a second, concurrently executing builder to ensure there is a
	// typed error returned indicating a build is in progress.
	//
	// When the second builder completes, having confirmed the error message
	// received is as expected.  It signals the first (blocked) builder that it
	// can now continue.

	// Thet test waits until the first builder notifies that it is done, and
	// has therefore ran its tests as well.

	var (
		pausedCh   = make(chan bool)
		continueCh = make(chan bool)
		doneCh     = make(chan bool)
	)

	// Build A
	builder1 := NewBuilder("builder1", true)
	builder1.buildFn = func(cfg *buildConfig, p v1.Platform) (d v1.Descriptor, l v1.Layer, err error) {
		if isFirstBuild(cfg, p) {
			pausedCh <- true // Notify of being paused
			<-continueCh     // Block until released
		}
		return
	}
	builder1.onDone = func() {
		doneCh <- true // Notify of being done
	}
	go func() {
		if err := builder1.Build(context.Background(), f, TestPlatforms); err != nil {
			fmt.Fprintf(os.Stderr, "test build error %v", err)
		}
	}()

	//  Wait until build 1 indicates it is paused
	<-pausedCh

	// Build B
	builder2 := NewBuilder("builder2", true)
	go func() {
		err = builder2.Build(context.Background(), f, TestPlatforms)
		if !errors.As(err, &ErrBuildInProgress{}) {
			fmt.Fprintf(os.Stderr, "test build error %v", err)
		}
	}()

	// Release the blocking Build A and wait until complete.
	continueCh <- true
	<-doneCh
}

func isFirstBuild(cfg *buildConfig, current v1.Platform) bool {
	first := cfg.platforms[0]
	return current.OS == first.OS &&
		current.Architecture == first.Architecture &&
		current.Variant == first.Variant
}

// ImageIndex represents the structure of an OCI Image Index.
type ImageIndex struct {
	SchemaVersion int `json:"schemaVersion"`
	Manifests     []struct {
		MediaType string `json:"mediaType"`
		Size      int64  `json:"size"`
		Digest    string `json:"digest"`
		Platform  struct {
			Architecture string `json:"architecture"`
			OS           string `json:"os"`
		} `json:"platform"`
	} `json:"manifests"`
}

// validateOCI performs a cursory check that the given path exists and
// has the basics of an OCI compliant structure.
func validateOCI(path string, t *testing.T) {
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("unable to stat output path. %v", path)
		return
	}

	ociLayoutFile := filepath.Join(path, "oci-layout")
	indexJSONFile := filepath.Join(path, "index.json")
	blobsDir := filepath.Join(path, "blobs")

	// Check if required files and directories exist
	if _, err := os.Stat(ociLayoutFile); os.IsNotExist(err) {
		t.Fatal("missing oci-layout file")
	}
	if _, err := os.Stat(indexJSONFile); os.IsNotExist(err) {
		t.Fatal("missing index.json file")
	}
	if _, err := os.Stat(blobsDir); os.IsNotExist(err) {
		t.Fatal("missing blobs directory")
	}

	// Load and validate index.json
	indexJSONData, err := os.ReadFile(indexJSONFile)
	if err != nil {
		t.Fatalf("failed to read index.json: %v", err)
	}

	var imageIndex ImageIndex
	err = json.Unmarshal(indexJSONData, &imageIndex)
	if err != nil {
		t.Fatalf("failed to parse index.json: %v", err)
	}

	if imageIndex.SchemaVersion != 2 {
		t.Fatalf("invalid schema version, expected 2, got %d", imageIndex.SchemaVersion)
	}

	// Additional validation of the Image Index structure can be added here
	// extract. for example checking that the path includes the README.md
	// and one of the binaries in the exact location expected (the data layer
	// blob and exec layer blob, respectively)
}
