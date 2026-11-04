// Package kafkaruntime embeds the language-agnostic Kafka runtime adapter
// binary into func and extracts it for local `func run` sub-process delivery.
//
// The adapter is the local counterpart of the on-cluster sidecar: it consumes
// Kafka and POSTs each record as a CloudEvent to the function over loopback
// HTTP, committing offsets only on a 2xx. For a local run func execs this
// binary as a child process next to the function, so Kafka works for every
// language with no in-process code.
//
// POC/WIP: only the host GOOS/GOARCH binary is embedded, populated by the
// Makefile `kafka-runtime-embed` step (which `go install`s the pinned adapter
// module). Embedding the full func release matrix is a TODO.
package kafkaruntime

import (
	"crypto/sha256"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Version is the pinned adapter release. It is the single source of truth for
// both the embedded local binary and the on-cluster image reference
// (pkg/k8s/deployer.go builds DefaultKafkaRuntimeImage from it), so a function
// runs the identical adapter locally and deployed.
const Version = "v0.1.0"

// ErrUnavailable is returned by Extract when no adapter binary is embedded for
// the current platform (e.g. a build that skipped this GOOS/GOARCH).
var ErrUnavailable = errors.New("local Kafka runtime unavailable on this platform")

// bin holds the per-platform adapter binaries. The directory always exists (a
// .gitkeep placeholder is committed); a build that did not run the embed step,
// or that targets a skipped platform, simply has no matching binary and
// Extract returns ErrUnavailable.
//
//go:embed all:bin
var binFS embed.FS

// binaryName is the embedded/extracted file name for the current platform.
func binaryName() string {
	name := fmt.Sprintf("fkafka-runtime_%s_%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// Extract writes the embedded adapter binary to func's config dir, keyed by
// version, and returns its path. An already-extracted binary with a matching
// checksum is reused; otherwise it is re-extracted. Returns ErrUnavailable when
// no binary is embedded for the current platform.
func Extract() (string, error) {
	data, err := binFS.ReadFile("bin/" + binaryName())
	if err != nil {
		return "", ErrUnavailable
	}

	dir := filepath.Join(configDir(), "kafka-runtime", Version)
	dst := filepath.Join(dir, binaryName())

	want := sha256.Sum256(data)
	if existing, err := os.ReadFile(dst); err == nil && sha256.Sum256(existing) == want {
		return dst, nil // cached, intact
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating kafka runtime dir: %w", err)
	}

	// Write to a temp file in the same dir, then rename for atomicity.
	tmp, err := os.CreateTemp(dir, binaryName()+".tmp-*")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", fmt.Errorf("writing kafka runtime binary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("closing kafka runtime binary: %w", err)
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return "", fmt.Errorf("chmod kafka runtime binary: %w", err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return "", fmt.Errorf("installing kafka runtime binary: %w", err)
	}
	return dst, nil
}

// configDir mirrors pkg/config.Dir without importing it: pkg/config depends on
// pkg/k8s, which depends on this package, so importing config here would create
// a cycle. Kept deliberately small; keep in sync with config.Dir.
func configDir() string {
	path := ""
	if home, err := os.UserHomeDir(); err == nil {
		path = filepath.Join(home, ".config", "func")
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		path = filepath.Join(xdg, "func")
	}
	return path
}
