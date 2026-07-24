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
func Delete(ctx context.Context, cfg ClusterConfig, out io.Writer) error {
	defer setKubeconfig(cfg.Kubeconfig())()

	if _, err := os.Stat(cfg.Kubeconfig()); err == nil {
		status(out, "Deleting Cluster")
		if err := run(ctx, out, "",
			cfg.kind(), "delete", "cluster",
			"--name="+cfg.Name,
			"--kubeconfig="+cfg.Kubeconfig()); err != nil {
			warnf(out, "failed to delete cluster %q: %v", cfg.Name, err)
		}
		_ = os.RemoveAll(filepath.Dir(cfg.Kubeconfig()))
	}

	if remaining := List(); len(remaining) > 0 {
		fmt.Fprintf(out, "Other func-managed cluster(s) still running: %v; leaving host registry config in place.\n",
			remaining)
	} else if !cfg.SkipRegistryConfig {
		revertHostRegistry(out)
	}

	fmt.Fprintf(out, "%s  Downloaded container images are not automatically removed.\n", red("NOTE:"))
	fmt.Fprintln(out, green("DONE"))

	return nil
}
