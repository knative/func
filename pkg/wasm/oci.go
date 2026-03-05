// Package wasm provides WASM/WASI build support for the func CLI.
package wasm

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"

	fn "knative.dev/func/pkg/functions"
)

const (
	// MediaTypeWasmConfig is the OCI config media type for WASM artifacts.
	MediaTypeWasmConfig = "application/vnd.wasm.config.v0+json"
	// MediaTypeWasmLayer is the OCI layer media type for raw WASM binaries.
	MediaTypeWasmLayer = "application/wasm"
)

// CredentialsProvider provides registry credentials for a given image reference.
type CredentialsProvider func(ctx context.Context, image string) (username, password string, err error)

// ociPusher is the default Pusher implementation that pushes a WASM binary as
// an OCI artifact to a registry using go-containerregistry.
type ociPusher struct {
	credentialsProvider CredentialsProvider
	transport           http.RoundTripper
	insecure            bool
	verbose             bool
}

// Push implements the Pusher interface. It reads the WASM binary at wasmPath,
// packages it as an OCI artifact with the correct WASM-specific media types
// (NOT a container image), and pushes it to the registry at imageRef.
//
// The resulting manifest looks like:
//
//	{
//	  "schemaVersion": 2,
//	  "mediaType": "application/vnd.oci.image.manifest.v1+json",
//	  "config": { "mediaType": "application/vnd.wasm.config.v0+json", ... },
//	  "layers": [{ "mediaType": "application/wasm", ... }]
//	}
func (p *ociPusher) Push(ctx context.Context, imageRef, wasmPath string) (digest string, err error) {
	// Read the raw WASM binary.
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return "", fmt.Errorf("reading wasm binary %q: %w", wasmPath, err)
	}

	if p.verbose {
		fmt.Fprintf(os.Stderr, "Packaging WASM artifact (%d bytes) as OCI artifact\n", len(wasmBytes))
	}

	// Build the OCI WASM artifact image.
	img, err := BuildWasmOCIArtifact(wasmBytes)
	if err != nil {
		return "", fmt.Errorf("building OCI WASM artifact: %w", err)
	}

	// Parse the image reference.
	nameOpts := []name.Option{}
	if p.insecure {
		nameOpts = append(nameOpts, name.Insecure)
	}
	ref, err := name.ParseReference(imageRef, nameOpts...)
	if err != nil {
		return "", fmt.Errorf("parsing image reference %q: %w", imageRef, err)
	}

	// Build remote options.
	remoteOpts := []remote.Option{
		remote.WithContext(ctx),
	}

	if p.transport != nil {
		remoteOpts = append(remoteOpts, remote.WithTransport(p.transport))
	}

	auth := buildAuthenticator(ctx, imageRef, p.credentialsProvider)
	remoteOpts = append(remoteOpts, remote.WithAuth(auth))

	if p.verbose {
		fmt.Fprintf(os.Stderr, "Pushing WASM OCI artifact to %s\n", imageRef)
	}

	// Push the artifact.
	if err = remote.Write(ref, img, remoteOpts...); err != nil {
		return "", fmt.Errorf("pushing OCI WASM artifact to %q: %w", imageRef, err)
	}

	// Return the image digest.
	d, err := img.Digest()
	if err != nil {
		return "", fmt.Errorf("getting image digest: %w", err)
	}
	return d.String(), nil
}

// PushFunction implements fn.Pusher for WASM functions.
// It locates the compiled .wasm binary via WasmBinaryPath, then delegates to
// the low-level Push(imageRef, wasmPath) to push the OCI WASM artifact.
func (p *ociPusher) PushFunction(ctx context.Context, f fn.Function) (string, error) {
	imageRef := f.Build.Image
	if imageRef == "" {
		return "", fmt.Errorf("function %q: %w", f.Name, ErrNoImageRef)
	}

	wasmPath, err := WasmBinaryPath(f)
	if err != nil {
		return "", fmt.Errorf("locating WASM binary for %q: %w", f.Name, err)
	}

	return p.Push(ctx, imageRef, wasmPath)
}

// wasmFnPusher wraps ociPusher and implements fn.Pusher.
type wasmFnPusher struct{ inner *ociPusher }

func (w *wasmFnPusher) Push(ctx context.Context, f fn.Function) (string, error) {
	return w.inner.PushFunction(ctx, f)
}

// PusherOpt is a functional option for the WASM fn.Pusher.
type PusherOpt func(*ociPusher)

// WithPusherVerbose enables verbose logging.
func WithPusherVerbose(verbose bool) PusherOpt {
	return func(p *ociPusher) { p.verbose = verbose }
}

// WithPusherCredentials sets the registry credentials provider.
func WithPusherCredentials(cp CredentialsProvider) PusherOpt {
	return func(p *ociPusher) { p.credentialsProvider = cp }
}

// WithPusherTransport sets the HTTP transport.
func WithPusherTransport(t http.RoundTripper) PusherOpt {
	return func(p *ociPusher) { p.transport = t }
}

// WithPusherInsecure disables TLS verification.
func WithPusherInsecure(insecure bool) PusherOpt {
	return func(p *ociPusher) { p.insecure = insecure }
}

// NewPusher creates a new WASM fn.Pusher with the given options.
func NewPusher(opts ...PusherOpt) fn.Pusher {
	p := &ociPusher{}
	for _, o := range opts {
		o(p)
	}
	return &wasmFnPusher{inner: p}
}

// BuildWasmOCIArtifact constructs a v1.Image that represents a WASM OCI artifact
// (not a container image). It uses WASM-specific media types for both the config
// and the single layer (raw .wasm binary bytes).
//
// The resulting manifest structure:
//
//	config:  application/vnd.wasm.config.v0+json
//	layer:   application/wasm  (raw bytes, NOT tarred or gzipped)
func BuildWasmOCIArtifact(wasmBytes []byte) (v1.Image, error) {
	// Start from an empty OCI image.
	img := empty.Image

	// Set the manifest media type to standard OCI image manifest.
	img = mutate.MediaType(img, types.OCIManifestSchema1)

	// Set the config media type to the WASM-specific type.
	img = mutate.ConfigMediaType(img, MediaTypeWasmConfig)

	// Set config to an empty ConfigFile (will serialize as minimal JSON).
	// The WASM config blob can be empty JSON or minimal metadata.
	img, err := mutate.ConfigFile(img, &v1.ConfigFile{})
	if err != nil {
		return nil, fmt.Errorf("setting WASM config: %w", err)
	}

	// Create the WASM layer: raw binary bytes with application/wasm media type.
	// The static layer is NOT compressed — it stores and pushes bytes as-is.
	wasmLayer := static.NewLayer(wasmBytes, MediaTypeWasmLayer)

	// Append the WASM layer.
	img, err = mutate.AppendLayers(img, wasmLayer)
	if err != nil {
		return nil, fmt.Errorf("appending WASM layer: %w", err)
	}

	return img, nil
}

// buildAuthenticator creates an authn.Authenticator from an optional credentials
// provider. Falls back to anonymous access if no provider is set.
func buildAuthenticator(ctx context.Context, imageRef string, provider CredentialsProvider) authn.Authenticator {
	if provider == nil {
		return authn.Anonymous
	}
	username, password, err := provider(ctx, imageRef)
	if err != nil || (username == "" && password == "") {
		return authn.Anonymous
	}
	return authn.FromConfig(authn.AuthConfig{
		Username: username,
		Password: password,
	})
}
