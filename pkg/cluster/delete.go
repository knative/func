package cluster

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Delete removes a single func-managed dev cluster. The shared registry
// container and the host's insecure-registries entry are removed only when
// the *last* func-managed cluster is being torn down — other surviving
// clusters keep using the shared registry.
func Delete(ctx context.Context, cfg ClusterConfig, out io.Writer) error {
	// Set KUBECONFIG for child processes; restore the caller's value on return.
	defer setKubeconfig(cfg.Kubeconfig())()

	status(out, "Deleting Cluster")

	if err := run(ctx, out, "",
		cfg.kind(), "delete", "cluster",
		"--name="+cfg.Name,
		"--kubeconfig="+cfg.Kubeconfig()); err != nil {
		warnf(out, "failed to delete cluster %q: %v", cfg.Name, err)
	}

	// Remove this cluster's kubeconfig dir so the "last cluster?" check
	// below reflects the post-delete state.
	_ = os.RemoveAll(filepath.Dir(cfg.Kubeconfig()))

	remaining := List()
	if len(remaining) == 0 {
		status(out, "Last func cluster removed; tearing down shared registry")
		teardownRegistry(ctx, cfg, out)
		if !cfg.SkipRegistryConfig {
			revertHostRegistry(out)
		}
	} else {
		fmt.Fprintf(out, "Registry left running; shared with %d other func-managed cluster(s): %v\n",
			len(remaining), remaining)
	}

	fmt.Fprintf(out, "%s  Downloaded container images are not automatically removed.\n", red("NOTE:"))
	fmt.Fprintln(out, green("DONE"))

	return nil
}
