package wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

const (
	// witDir is the directory within a function root that contains WIT files.
	witDir = "wit"

	// witVersionsFile is the marker file recording the last-provisioned
	// builderImages state. Written after successful WIT provisioning.
	witVersionsFile = ".versions"
)

// ProvisionWIT downloads WIT dependencies declared in builderImages into
// wit/<key>/ subdirectories within the function root. Each OCI artifact is
// pulled via go-containerregistry, saved to a temp file, and extracted via
// `wasm-tools component wit --out-dir wit/<key>/`.
//
// The function skips work entirely when wit/.versions matches the current
// builderImages map. Only changed entries are re-provisioned (stale subdirs
// are removed before re-downloading).
//
// ProvisionWIT does NOT touch wit/world.wit — that file is owned by the user
// (part of the template). Each downloaded subdir receives a .gitignore with
// "*" to prevent accidental commits.
func ProvisionWIT(
	ctx context.Context,
	root string,
	builderImages map[string]string,
	verbose bool,
) error {
	if len(builderImages) == 0 {
		return nil
	}

	witPath := filepath.Join(root, witDir)

	// Load the current .versions marker (if any).
	current, err := loadVersions(witPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading WIT versions marker: %w", err)
	}

	// Determine which keys need provisioning.
	toProvision := diffVersions(current, builderImages)
	if len(toProvision) == 0 {
		if verbose {
			fmt.Fprintf(os.Stderr, "WIT deps up-to-date, skipping download\n")
		}
		return nil
	}

	// Verify wasm-tools is available before doing any network work.
	wasmToolsPath, err := exec.LookPath("wasm-tools")
	if err != nil {
		return fmt.Errorf("%w: wasm-tools not found on PATH (install from https://github.com/bytecodealliance/wasm-tools)", ErrToolchainNotFound)
	}

	// Ensure the wit/ directory exists.
	if err := os.MkdirAll(witPath, 0755); err != nil {
		return fmt.Errorf("creating wit directory: %w", err)
	}

	for key, ociRef := range toProvision {
		subDir := filepath.Join(witPath, key)

		// Remove stale subdir if it exists (version changed).
		if _, statErr := os.Stat(subDir); statErr == nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "Removing stale WIT dep %s\n", key)
			}
			if err := os.RemoveAll(subDir); err != nil {
				return fmt.Errorf("removing stale WIT dep %q: %w", key, err)
			}
		}

		if verbose {
			fmt.Fprintf(os.Stderr, "Downloading WIT dep %q from %s\n", key, ociRef)
		}

		if err := downloadAndExtractWIT(ctx, wasmToolsPath, ociRef, subDir, verbose); err != nil {
			return fmt.Errorf("%w: dep %q from %s: %v", ErrWITProvisionFailed, key, ociRef, err)
		}

		// Write .gitignore to prevent accidental commits of downloaded artifacts.
		if err := writeGitignore(subDir); err != nil {
			return fmt.Errorf("writing .gitignore for WIT dep %q: %w", key, err)
		}

		if verbose {
			fmt.Fprintf(os.Stderr, "WIT dep %q provisioned at %s\n", key, subDir)
		}
	}

	// Merge newly provisioned entries into current state.
	for k, v := range toProvision {
		current[k] = v
	}

	// Write the updated .versions marker.
	if err := saveVersions(witPath, current); err != nil {
		return fmt.Errorf("writing WIT versions marker: %w", err)
	}

	return nil
}

// downloadAndExtractWIT pulls a WIT OCI artifact and extracts its WIT files
// into outDir using wasm-tools.
//
// WASI WIT OCI artifacts are published as single-layer Wasm components
// (media type application/wasm) containing embedded WIT definitions.
// wasm-tools component wit can extract these WIT files from the component.
func downloadAndExtractWIT(ctx context.Context, wasmToolsPath, ociRef, outDir string, verbose bool) error {
	// Pull the OCI artifact layer bytes.
	wasmBytes, err := pullOCILayer(ctx, ociRef, verbose)
	if err != nil {
		return err
	}

	// Write the .wasm bytes to a temp file for wasm-tools.
	tmpFile, err := os.CreateTemp("", "wit-*.wasm")
	if err != nil {
		return fmt.Errorf("creating temp wasm file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(wasmBytes); err != nil {
		tmpFile.Close()
		return fmt.Errorf("writing temp wasm file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing temp wasm file: %w", err)
	}

	// Create the output directory.
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("creating output directory %s: %w", outDir, err)
	}

	// Run: wasm-tools component wit <tmpFile> --out-dir <outDir>
	args := []string{"component", "wit", tmpFile.Name(), "--out-dir", outDir}
	cmd := exec.CommandContext(ctx, wasmToolsPath, args...)
	if verbose {
		fmt.Fprintf(os.Stderr, "wasm-tools %s\n", strings.Join(args, " "))
		cmd.Stdout = os.Stderr
	}
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wasm-tools component wit failed: %w\n%s", err, stderrBuf.String())
	}

	return nil
}

// pullOCILayer fetches the first layer of a single-layer OCI artifact.
// WASI WIT artifacts have exactly one layer with media type application/wasm.
func pullOCILayer(ctx context.Context, ociRef string, verbose bool) ([]byte, error) {
	ref, err := name.ParseReference(ociRef)
	if err != nil {
		return nil, fmt.Errorf("parsing OCI reference %q: %w", ociRef, err)
	}

	remoteOpts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Pulling OCI artifact %s\n", ociRef)
	}

	img, err := remote.Image(ref, remoteOpts...)
	if err != nil {
		return nil, fmt.Errorf("%w: %q: %v (check network connectivity and registry authentication)", ErrOCIPullFailed, ociRef, err)
	}

	layers, err := img.Layers()
	if err != nil {
		return nil, fmt.Errorf("%w: reading layers from %q: %v", ErrOCIPullFailed, ociRef, err)
	}

	if len(layers) == 0 {
		return nil, fmt.Errorf("%w: %q has no layers", ErrOCIPullFailed, ociRef)
	}

	// Read the first (and typically only) layer.
	reader, err := layers[0].Uncompressed()
	if err != nil {
		return nil, fmt.Errorf("%w: reading layer from %q: %v", ErrOCIPullFailed, ociRef, err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("%w: reading layer data from %q: %v", ErrOCIPullFailed, ociRef, err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Pulled OCI artifact %s (%d bytes)\n", ociRef, len(data))
	}

	return data, nil
}

// loadVersions reads the .versions marker file from witPath and returns the
// recorded builderImages map. Returns an empty map (not an error) if the file
// does not exist.
func loadVersions(witPath string) (map[string]string, error) {
	versionsPath := filepath.Join(witPath, witVersionsFile)
	data, err := os.ReadFile(versionsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return nil, err
	}

	var versions map[string]string
	if err := json.Unmarshal(data, &versions); err != nil {
		// Corrupt marker — treat as missing, re-provision everything.
		return make(map[string]string), nil
	}
	return versions, nil
}

// saveVersions writes the builderImages map as sorted JSON to the .versions
// marker file in witPath. Sorted keys ensure deterministic output for diffs.
func saveVersions(witPath string, versions map[string]string) error {
	// Sort keys for deterministic output.
	keys := make([]string, 0, len(versions))
	for k := range versions {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build sorted map for JSON marshaling.
	sorted := make(map[string]string, len(versions))
	for _, k := range keys {
		sorted[k] = versions[k]
	}

	data, err := json.MarshalIndent(sorted, "", "  ")
	if err != nil {
		return err
	}

	versionsPath := filepath.Join(witPath, witVersionsFile)
	return os.WriteFile(versionsPath, append(data, '\n'), 0644)
}

// diffVersions returns the subset of desired that differs from current.
// An entry is included if its value differs or if it's absent in current.
// Stale entries (in current but not desired) are not returned — they are
// handled by ProvisionWIT which only processes keys present in builderImages.
func diffVersions(current, desired map[string]string) map[string]string {
	diff := make(map[string]string)
	for k, v := range desired {
		if current[k] != v {
			diff[k] = v
		}
	}
	return diff
}

// writeGitignore creates a .gitignore file with "*" in the given directory,
// preventing downloaded WIT artifacts from being accidentally committed.
func writeGitignore(dir string) error {
	return os.WriteFile(
		filepath.Join(dir, ".gitignore"),
		[]byte("# Downloaded WIT artifacts — do not commit\n*\n"),
		0644,
	)
}
