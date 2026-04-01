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
// wit/deps/<pkg>/ subdirectories within the function root. Each OCI artifact
// is pulled via go-containerregistry, saved to a temp file, extracted via
// `wasm-tools component wit --out-dir <tmpDir>`, and then restructured into
// the standard WIT package layout expected by WIT resolvers:
//
//	wit/
//	  world.wit          ← user-owned root (template file)
//	  deps/
//	    <pkg>/           ← one dir per WIT package
//	      <pkg>.wit
//	  .versions          ← version marker
//
// wasm-tools extracts a nested layout (main .wit + deps/ subdir containing
// transitive dependencies). We flatten ALL of them (the main package AND its
// transitive deps) as siblings into wit/deps/, each in its own directory.
// Only the specific package dirs produced by extraction are replaced — any
// user-vendored deps in wit/deps/ are preserved.
//
// The function skips work entirely when wit/.versions matches the current
// builderImages map. Only changed entries are re-provisioned.
//
// ProvisionWIT does NOT touch wit/world.wit — that file is owned by the user
// (part of the template). Each provisioned package dir receives a .gitignore
// with "*" to prevent accidental commits.
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

	// Ensure the wit/deps/ directory exists.
	depsDir := filepath.Join(witPath, "deps")
	if err := os.MkdirAll(depsDir, 0755); err != nil {
		return fmt.Errorf("creating wit/deps directory: %w", err)
	}

	for key, ociRef := range toProvision {
		if verbose {
			fmt.Fprintf(os.Stderr, "Downloading WIT dep %q from %s\n", key, ociRef)
		}

		// Extract to a temp dir first — wasm-tools produces:
		//   <tmpDir>/<pkg>.wit       (main package file)
		//   <tmpDir>/deps/*.wit      (transitive dependencies)
		// We flatten ALL of them into wit/deps/<name>/<name>.wit.
		tmpDir, mkErr := os.MkdirTemp("", "wit-extract-*")
		if mkErr != nil {
			return fmt.Errorf("creating temp dir for WIT extraction: %w", mkErr)
		}

		if err := downloadAndExtractWIT(ctx, wasmToolsPath, ociRef, tmpDir, verbose); err != nil {
			os.RemoveAll(tmpDir)
			return fmt.Errorf("%w: dep %q from %s: %v", ErrWITProvisionFailed, key, ociRef, err)
		}

		// Restructure: flatten main + transitive deps into wit/deps/<pkg>/.
		// Only the specific package dirs produced are replaced — user-vendored
		// deps are preserved.
		provisioned, err := restructureWITDeps(tmpDir, depsDir, verbose)
		os.RemoveAll(tmpDir)
		if err != nil {
			return fmt.Errorf("restructuring WIT dep %q: %w", key, err)
		}

		// Write .gitignore in each provisioned dir.
		for _, pkgDir := range provisioned {
			if wErr := writeGitignore(pkgDir); wErr != nil {
				return fmt.Errorf("writing .gitignore for WIT dep %q: %w", filepath.Base(pkgDir), wErr)
			}
		}

		if verbose {
			fmt.Fprintf(os.Stderr, "WIT dep %q provisioned into %s\n", key, depsDir)
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

// restructureWITDeps takes the wasm-tools extraction output (in srcDir) and
// flattens .wit files into dstDir/<name>/<name>.wit.
//
// wasm-tools produces:
//
//	<srcDir>/<main>.wit          ← the requested package
//	<srcDir>/deps/<dep>.wit      ← each transitive dependency
//
// The WIT resolver expects all packages as flat siblings:
//
//	<dstDir>/<main>/<main>.wit
//	<dstDir>/<dep>/<dep>.wit
//
// Main packages (from srcDir root) always overwrite existing dirs.
// Transitive deps (from srcDir/deps/) are written only if the target dir
// does NOT already exist — this prevents a smaller transitive stub from
// clobbering a richer package that was already provisioned from its own
// builderImages entry (e.g., HTTP's transitive cli.wit overwriting the
// full CLI package).
//
// Returns the list of destination directories that were created/updated.
func restructureWITDeps(srcDir, dstDir string, verbose bool) ([]string, error) {
	// --- Main packages: always overwrite ---
	mainWits, err := filepath.Glob(filepath.Join(srcDir, "*.wit"))
	if err != nil {
		return nil, fmt.Errorf("listing main WIT files: %w", err)
	}
	provisioned := make([]string, 0, len(mainWits))
	for _, witFile := range mainWits {
		baseName := filepath.Base(witFile)
		pkgName := strings.TrimSuffix(baseName, ".wit")
		pkgDir := filepath.Join(dstDir, pkgName)

		if err := os.RemoveAll(pkgDir); err != nil {
			return nil, fmt.Errorf("removing stale pkg dir %q: %w", pkgName, err)
		}
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			return nil, fmt.Errorf("creating pkg dir %q: %w", pkgName, err)
		}
		if err := copyFile(witFile, filepath.Join(pkgDir, baseName)); err != nil {
			return nil, fmt.Errorf("copying %s: %w", baseName, err)
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "  wit/deps/%s/%s\n", pkgName, baseName)
		}
		provisioned = append(provisioned, pkgDir)
	}

	// --- Transitive deps: only fill gaps (don't overwrite) ---
	depsSubDir := filepath.Join(srcDir, "deps")
	if info, statErr := os.Stat(depsSubDir); statErr == nil && info.IsDir() {
		depWits, err := filepath.Glob(filepath.Join(depsSubDir, "*.wit"))
		if err != nil {
			return nil, fmt.Errorf("listing dep WIT files: %w", err)
		}
		for _, witFile := range depWits {
			baseName := filepath.Base(witFile)
			pkgName := strings.TrimSuffix(baseName, ".wit")
			pkgDir := filepath.Join(dstDir, pkgName)

			// Skip if this package already exists (provisioned by another
			// builderImages entry or a previous main package).
			if _, statErr := os.Stat(pkgDir); statErr == nil {
				if verbose {
					fmt.Fprintf(os.Stderr, "  wit/deps/%s/%s (skipped, already exists)\n", pkgName, baseName)
				}
				continue
			}

			if err := os.MkdirAll(pkgDir, 0755); err != nil {
				return nil, fmt.Errorf("creating dep dir %q: %w", pkgName, err)
			}
			if err := copyFile(witFile, filepath.Join(pkgDir, baseName)); err != nil {
				return nil, fmt.Errorf("copying dep %s: %w", baseName, err)
			}
			if verbose {
				fmt.Fprintf(os.Stderr, "  wit/deps/%s/%s\n", pkgName, baseName)
			}
			provisioned = append(provisioned, pkgDir)
		}
	}

	return provisioned, nil
}

// copyFile copies a single file from src to dst, preserving permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
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
