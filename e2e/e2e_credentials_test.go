//go:build e2e
// +build e2e

package e2e

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

// Test that FUNC_USERNAME/FUNC_PASSWORD are picked up by all pushers
// by asserting the pusher's log message includes the provided username.
// This test runs for host, pack, and s2i builders automatically.
func TestCredentials_DockerPusher_EnvUsed(t *testing.T) {
	for _, builder := range []string{"host", "pack", "s2i"} {
		t.Run(builder, func(t *testing.T) {
			name := fmt.Sprintf("func-e2e-creds-docker-%s", builder)
			_ = fromCleanEnv(t, name)

			// Provide credentials via env (what issue #3314 validates)
			const user = "e2euser"
			const pass = "e2epass"
			os.Setenv("FUNC_USERNAME", user)
			os.Setenv("FUNC_PASSWORD", pass)

			// Init a simple function
			if err := newCmd(t, "init", "-l=go").Run(); err != nil {
				t.Fatal(err)
			}

			// Build and push; capture stderr to assert credentials log
			cmd := newCmd(t, "build", "--builder", builder, "--push", "--registry", Registry)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("build --push failed: %v\nerrOut: %s", err, stderr.String())
			}

			// docker pusher logs: Pushing function image to the registry %q using the %q user credentials
			if !strings.Contains(stderr.String(), "using the \""+user+"\" user credentials") {
				t.Fatalf("expected docker pusher to log username %q in stderr, got: %s", user, stderr.String())
			}

			// Clean local images/volumes to keep CI tidy
			cleanImages(t, name)
		})
	}
}
