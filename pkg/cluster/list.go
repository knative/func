package cluster

import (
	"path/filepath"
	"strings"

	"knative.dev/func/pkg/config"
)

// List returns the names of func-managed clusters on this host — those
// with a kubeconfig under <config.Dir()>/clusters/<name>.local/. Kind
// clusters created outside func (e.g. via `kind create cluster` directly)
// are not included; this is intentional so that `func cluster delete foo`
// can't accidentally remove an unrelated kind cluster, and so the list
// reflects only what this tool manages.
//
// No error return: a missing/unreadable clusters directory means "none",
// which is the correct answer for a fresh system.
func List() []string {
	matches, err := filepath.Glob(filepath.Join(config.Dir(), "clusters", "*.local", "kubeconfig.yaml"))
	if err != nil {
		return nil
	}
	var names []string
	for _, m := range matches {
		// m = .../clusters/<name>.local/kubeconfig.yaml
		dir := filepath.Base(filepath.Dir(m))
		names = append(names, strings.TrimSuffix(dir, ".local"))
	}
	return names
}
