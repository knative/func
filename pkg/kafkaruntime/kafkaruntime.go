// Package kafkaruntime provides the Kafka runtime that local `func run`
// executes next to a function configured with run.kafka.
//
// The runtime (knative.dev/func-adapter-kafka) is the same program that runs as
// the Kafka sidecar on the cluster: it consumes records and POSTs each one as a
// CloudEvent to the function over HTTP, committing the offset only on a 2xx.
// Locally it runs as a child process of `func run` instead of a sidecar
// container, so Kafka delivery works for every language without any Kafka code
// in the function.
//
// The binary is built from the pinned runtime module (Version) by
// hack/kafka-runtime.sh, which the Makefile runs before building func for a
// platform, and is embedded per GOOS/GOARCH from bin/<goos>_<goarch>. A func
// built without that step (a plain `go build`, or a program importing func as a
// library), or for a platform without a bin directory, embeds no binary: Binary
// then returns ErrUnavailable unless BinaryEnv names one.
package kafkaruntime

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// Version is the pinned Kafka runtime release. It is the single source of
// truth for both the binary embedded for local `func run` and the image the
// cluster deployers inject as the sidecar (k8s.DefaultKafkaRuntimeImage), so a
// function runs the identical runtime locally and deployed.
const Version = "v0.2.0"

// BinaryEnv names an environment variable which, when set, is the path of a
// Kafka runtime binary to run instead of the embedded one: for testing a
// locally built runtime, or for platforms whose func build embeds none.
const BinaryEnv = "FUNC_KAFKA_RUNTIME_BINARY"

// ErrUnavailable is returned by Binary when this func build embeds no Kafka
// runtime for the current platform and BinaryEnv is not set.
var ErrUnavailable = errors.New("no Kafka runtime is embedded for this platform")

// versionFile is written next to the embedded binary by hack/kafka-runtime.sh
// and records the runtime version it was built from.
const versionFile = "VERSION"

// Binary returns the path of the Kafka runtime executable: the path named by
// BinaryEnv if set, otherwise the embedded binary, extracted into Dir on first
// use and reused while its content matches.
func Binary() (string, error) {
	if p := strings.TrimSpace(os.Getenv(BinaryEnv)); p != "" {
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("%s: %w", BinaryEnv, err)
		}
		return p, nil
	}
	return extract(embedded, embedDir, Dir())
}

// Dir returns the directory the embedded binary is extracted into: a
// version-keyed directory under func's config directory, or "" when no config
// directory can be derived.
//
// The config directory is derived as config.Dir derives it. This package
// cannot import pkg/config (pkg/config depends on pkg/k8s, which depends on
// this package for Version), so a test in pkg/config keeps the two in step.
func Dir() string {
	base := ""
	if home, err := os.UserHomeDir(); err == nil {
		base = filepath.Join(home, ".config", "func")
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		base = filepath.Join(xdg, "func")
	}
	if base == "" {
		return ""
	}
	return filepath.Join(base, "kafka-runtime", Version)
}

// binaryName is the file name of the runtime binary, both within the embedded
// directory and once extracted.
func binaryName() string {
	if runtime.GOOS == "windows" {
		return "fkafka-runtime.exe"
	}
	return "fkafka-runtime"
}

// extract writes the binary embedded in fsys under src into dst and returns
// its path. A binary already at that path with identical content is reused.
func extract(fsys fs.FS, src, dst string) (string, error) {
	data, err := fs.ReadFile(fsys, path.Join(src, binaryName()))
	if errors.Is(err, fs.ErrNotExist) {
		return "", ErrUnavailable
	} else if err != nil {
		return "", fmt.Errorf("reading the embedded Kafka runtime: %w", err)
	}

	// Refuse a binary built from a different version than the one pinned (a
	// development build whose bin directory predates a Version bump), so local
	// runs never silently diverge from the image deployed to the cluster.
	stamp, err := fs.ReadFile(fsys, path.Join(src, versionFile))
	if err != nil {
		return "", fmt.Errorf("the embedded Kafka runtime has no version stamp; rebuild func with make: %w", err)
	}
	if v := strings.TrimSpace(string(stamp)); v != Version {
		return "", fmt.Errorf("the embedded Kafka runtime is %s but func pins %s; rebuild func with make", v, Version)
	}

	if dst == "" {
		return "", errors.New("cannot determine the func config directory to extract the Kafka runtime into")
	}
	target := filepath.Join(dst, binaryName())

	if existing, err := os.ReadFile(target); err == nil && sha256.Sum256(existing) == sha256.Sum256(data) {
		return target, nil
	}

	if err := os.MkdirAll(dst, 0o755); err != nil {
		return "", fmt.Errorf("creating the Kafka runtime directory: %w", err)
	}
	// Write to a temporary file and rename it into place, so a concurrent run
	// never executes a partially written binary.
	tmp, err := os.CreateTemp(dst, binaryName()+".tmp-*")
	if err != nil {
		return "", fmt.Errorf("extracting the Kafka runtime: %w", err)
	}
	defer os.Remove(tmp.Name()) // no-op once renamed
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", fmt.Errorf("extracting the Kafka runtime: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("extracting the Kafka runtime: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return "", fmt.Errorf("extracting the Kafka runtime: %w", err)
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		return "", fmt.Errorf("extracting the Kafka runtime: %w", err)
	}
	return target, nil
}
