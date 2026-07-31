package cluster

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Delete removes a func-managed dev cluster. The in-cluster registry is
// destroyed automatically with the Kind cluster. Host-side trust config
// (insecure-registries) is only reverted when this is the last func cluster,
// since other surviving clusters share the same host entry.
//
// Delete is safe when nothing is tracked (empty List / no kubeconfig): it
// still runs empty-list host-trust teardown and returns nil (idempotent,
// rm -f style). kind-delete failures are only warned when a kubeconfig was
// present — otherwise every empty-system delete would print a scary
// "failed to delete cluster" for a cluster that never existed.
func Delete(ctx context.Context, cfg ClusterConfig, out io.Writer) error {
	// Set KUBECONFIG for child processes; restore the caller's value on return.
	defer setKubeconfig(cfg.Kubeconfig())()

	status(out, "Deleting Cluster")

	_, kubeconfigErr := os.Stat(cfg.Kubeconfig())
	kubeconfigPresent := kubeconfigErr == nil

	if err := run(ctx, out, "",
		cfg.kind(), "delete", "cluster",
		"--name="+cfg.Name,
		"--kubeconfig="+cfg.Kubeconfig()); err != nil {
		if kubeconfigPresent {
			warnf(out, "failed to delete cluster %q: %v", cfg.Name, err)
		}
	}

	// Remove this cluster's kubeconfig dir so the "last cluster?" check
	// below reflects the post-delete state.
	_ = os.RemoveAll(filepath.Dir(cfg.Kubeconfig()))

	if remaining := List(); len(remaining) > 0 {
		fmt.Fprintf(out, "Other func-managed cluster(s) still running: %v; leaving host registry config in place.\n",
			remaining)
	} else if !cfg.SkipRegistryConfig {
		status(out, "Last func cluster removed; reverting host registry trust")
		revertHostRegistry(out)
	}

	fmt.Fprintf(out, "%s  Downloaded container images are not automatically removed.\n", red("NOTE:"))
	fmt.Fprintln(out, green("DONE"))

	return nil
}
