package oci

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	fn "knative.dev/func/pkg/functions"
	. "knative.dev/func/pkg/testing"
)

// TestBuilder ensures that, when given a Go Function, an OCI-compliant
// directory structure is created on .Build in the expected path.
func TestBuilder(t *testing.T) {
	root, done := Mktemp(t)
	defer done()

	client := fn.New()

	f, err := client.Init(fn.Function{Root: root, Runtime: "go"})
	if err != nil {
		t.Fatal(err)
	}

	builder := NewBuilder("", client, true)

	if err := builder.Build(context.Background(), f); err != nil {
		t.Fatal(err)
	}

	last := path(f.Root, fn.RunDataDir, "builds", "last", "oci")

	validateOCI(last, t)
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
