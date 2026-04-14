package cluster

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// setKubeconfig sets the KUBECONFIG environment variable and returns a
// function that restores the previous value. Every child process spawned
// via `run` inherits the parent env, so all kubectl/kind calls pick this
// up without further plumbing.
func setKubeconfig(path string) (restore func()) {
	prev, hadPrev := os.LookupEnv("KUBECONFIG")
	os.Setenv("KUBECONFIG", path)
	return func() {
		if hadPrev {
			os.Setenv("KUBECONFIG", prev)
		} else {
			os.Unsetenv("KUBECONFIG")
		}
	}
}

// run executes a command, optionally piping stdin. An empty stdin leaves
// the child's stdin unattached. The child inherits the parent process
// environment — notably KUBECONFIG, which Create/Delete set via
// setKubeconfig with deferred restore.
func run(ctx context.Context, out io.Writer, stdin string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = out
	cmd.Stderr = out
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", filepath.Base(name), strings.Join(args, " "), err)
	}
	return nil
}

// runOutput executes a command and returns its stdout as a trimmed string.
func runOutput(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", filepath.Base(name), strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// applyManifest pipes a YAML manifest string to kubectl apply -f -.
func applyManifest(ctx context.Context, out io.Writer, cfg ClusterConfig, manifest string) error {
	return run(ctx, out, manifest, cfg.kubectl(), "apply", "-f", "-")
}

// applyURL downloads a YAML document from the given URL and applies it via kubectl.
func applyURL(ctx context.Context, out io.Writer, cfg ClusterConfig, url string) error {
	body, err := httpGet(ctx, url)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	return run(ctx, out, body, cfg.kubectl(), "apply", "-f", "-")
}

// applyURLServerSide downloads YAML and applies it with --server-side.
func applyURLServerSide(ctx context.Context, out io.Writer, cfg ClusterConfig, url string) error {
	body, err := httpGet(ctx, url)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	return run(ctx, out, body, cfg.kubectl(), "apply", "--server-side", "-f", "-")
}

// httpGet fetches a URL and returns the response body as a string.
func httpGet(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// wait sleeps for the given duration, respecting context cancellation.
func wait(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
