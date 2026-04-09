package wasm_test

import (
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"

	"knative.dev/func/pkg/wasm"
)

// TestBuildWasmOCIArtifact_ManifestMediaType verifies that the produced OCI
// image uses the standard OCI manifest media type (not Docker).
func TestBuildWasmOCIArtifact_ManifestMediaType(t *testing.T) {
	t.Parallel()

	img := mustBuildArtifact(t, []byte("fake wasm bytes"))

	mt, err := img.MediaType()
	if err != nil {
		t.Fatalf("MediaType(): %v", err)
	}
	if mt != types.OCIManifestSchema1 {
		t.Errorf("manifest media type = %q, want %q", mt, types.OCIManifestSchema1)
	}
}

// TestBuildWasmOCIArtifact_ConfigMediaType verifies that the config blob uses
// the WASM-specific config media type.
func TestBuildWasmOCIArtifact_ConfigMediaType(t *testing.T) {
	t.Parallel()

	img := mustBuildArtifact(t, []byte("fake wasm bytes"))

	manifest, err := img.Manifest()
	if err != nil {
		t.Fatalf("Manifest(): %v", err)
	}
	want := types.MediaType(wasm.MediaTypeWasmConfig)
	if manifest.Config.MediaType != want {
		t.Errorf("config media type = %q, want %q", manifest.Config.MediaType, want)
	}
}

// TestBuildWasmOCIArtifact_SingleLayer verifies that the image has exactly one
// layer with the WASM-specific layer media type.
func TestBuildWasmOCIArtifact_SingleLayer(t *testing.T) {
	t.Parallel()

	img := mustBuildArtifact(t, []byte("fake wasm bytes"))

	manifest, err := img.Manifest()
	if err != nil {
		t.Fatalf("Manifest(): %v", err)
	}
	if len(manifest.Layers) != 1 {
		t.Fatalf("expected exactly 1 layer, got %d", len(manifest.Layers))
	}
	want := types.MediaType(wasm.MediaTypeWasmLayer)
	if manifest.Layers[0].MediaType != want {
		t.Errorf("layer media type = %q, want %q", manifest.Layers[0].MediaType, want)
	}
}

// TestBuildWasmOCIArtifact_LayerContents verifies that the single layer
// contains exactly the raw WASM bytes (not tarred or gzipped).
func TestBuildWasmOCIArtifact_LayerContents(t *testing.T) {
	t.Parallel()

	wasmBytes := []byte("\x00asm\x01\x00\x00\x00") // valid WASM magic + version
	img := mustBuildArtifact(t, wasmBytes)

	layers, err := img.Layers()
	if err != nil {
		t.Fatalf("Layers(): %v", err)
	}
	if len(layers) != 1 {
		t.Fatalf("expected 1 layer, got %d", len(layers))
	}

	// Verify digest matches the raw bytes (not compressed).
	manifest, err := img.Manifest()
	if err != nil {
		t.Fatalf("Manifest(): %v", err)
	}
	layerDesc := manifest.Layers[0]
	if int(layerDesc.Size) != len(wasmBytes) {
		t.Errorf("layer size = %d, want %d (raw WASM size — not compressed)", layerDesc.Size, len(wasmBytes))
	}
}

// TestBuildWasmOCIArtifact_NoContainerLayers verifies that the image has no
// compressed tar layers (no filesystem layers, no base image).
func TestBuildWasmOCIArtifact_NoContainerLayers(t *testing.T) {
	t.Parallel()

	img := mustBuildArtifact(t, []byte("fake wasm bytes"))

	manifest, err := img.Manifest()
	if err != nil {
		t.Fatalf("Manifest(): %v", err)
	}
	for i, layer := range manifest.Layers {
		if layer.MediaType == types.OCILayer ||
			layer.MediaType == types.OCIUncompressedLayer ||
			layer.MediaType == types.DockerManifestSchema2 {
			t.Errorf("layer[%d] has container media type %q — WASM artifacts must not contain container layers", i, layer.MediaType)
		}
	}
}

// mustBuildArtifact is a test helper that calls the exported BuildWasmOCIArtifact
// (or its equivalent via wasm package) and fatals on error.
func mustBuildArtifact(t *testing.T, wasmBytes []byte) v1.Image {
	t.Helper()
	img, err := wasm.BuildWasmOCIArtifact(wasmBytes)
	if err != nil {
		t.Fatalf("BuildWasmOCIArtifact(): %v", err)
	}
	return img
}
