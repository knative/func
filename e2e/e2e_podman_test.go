//go:build e2e
// +build e2e

package e2e

import (
	"testing"
)

// TestPodman_Pack ensures that the Podman container engine can be used to
// deploy functions built with Pack.
func TestPodman_Pack(t *testing.T) {
	skipUnlessPodmanEnabled(t) // naming suggestions welcome
	name := "func-e2e-test-podman-pack"
	_ = fromCleanEnv(t, name)
	if err := setupPodman(t); err != nil {
		t.Fatal(err)
	}

	// Create a Function
	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	// Deploy
	// ------
	if err := newCmd(t, "deploy", "--builder=pack").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	if !waitFor(t, ksvcUrl(name)) {
		t.Fatal("function did not deploy correctly")
	}
}

// TestPodman_S2I ensures that the Podman container engine can be used to
// deploy functions built with S2I.
func TestPodman_S2I(t *testing.T) {
	skipUnlessPodmanEnabled(t) // naming suggestions welcome
	name := "func-e2e-test-podman-s2i"
	_ = fromCleanEnv(t, name)
	if err := setupPodman(t); err != nil {
		t.Fatal(err)
	}

	// Create a Function
	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	// Deploy
	// ------
	if err := newCmd(t, "deploy", "--builder=s2i").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	if !waitFor(t, ksvcUrl(name)) {
		t.Fatal("function did not deploy correctly")
	}
}

func skipUnlessPodmanEnabled(t *testing.T) {
	if !Podman {
		t.Skip("Podman tests not enabled. Enable with FUNC_E2E_PODMAN=true and set FUNC_E2E_PODMAN_HOST to the Podman socket")
	}
	if PodmanHost == "" {
		t.Skip("FUNC_E2E_PODMAN_HOST must be set to the Podman socket path")
	}
}
