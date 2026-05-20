package cluster

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sync/errgroup"
)

// Create sets up a local kind development cluster with the configured components.
// This is the Go equivalent of hack/cluster.sh + hack/binaries.sh + hack/registry.sh.
func Create(ctx context.Context, cfg ClusterConfig, out io.Writer) error {
	// Set KUBECONFIG for child processes; restore the caller's value on return.
	defer setKubeconfig(cfg.Kubeconfig())()

	// Ensure directory exists for the final kubeconfig
	if err := os.MkdirAll(filepath.Dir(cfg.Kubeconfig()), 0o755); err != nil {
		return fmt.Errorf("creating kubeconfig directory: %w", err)
	}

	// Phase 0: Ensure required binaries
	if !cfg.SkipBinaries {
		if err := ensureBins(ctx, cfg, out); err != nil {
			return fmt.Errorf("binary setup: %w", err)
		}
	}

	if cfg.Retries > 1 {
		return allocateWithRetry(ctx, cfg, out)
	}
	return allocateCluster(ctx, cfg, out)
}

// allocationRetryBackoff is the wait between failed allocation attempts.
// Used with --retry for flaky CI environments.
const allocationRetryBackoff = 5 * time.Minute

func allocateWithRetry(ctx context.Context, cfg ClusterConfig, out io.Writer) error {
	statusf(out, "Cluster allocation will retry up to %d time(s)", cfg.Retries)

	for attempt := 1; attempt <= cfg.Retries; attempt++ {
		statusf(out, "------------------ Attempt %d ------------------", attempt)

		if err := allocateCluster(ctx, cfg, out); err == nil {
			return nil
		}
		if attempt < cfg.Retries {
			fmt.Fprintln(out, yellow("------------------ Sleeping for 5 minutes before retry ------------------"))
			if err := wait(ctx, allocationRetryBackoff); err != nil {
				return err
			}
		}
	}

	return fmt.Errorf("cluster allocation failed after %d attempt(s)", cfg.Retries)
}

// allocateCluster runs the actual install pipeline. The named return is
// load-bearing: allocationCleanup runs in a defer and needs to see the
// final err value.
func allocateCluster(ctx context.Context, cfg ClusterConfig, out io.Writer) (err error) {
	defer func() { allocationCleanup(ctx, cfg, out, err) }()

	// Phase 1: Sequential prerequisites — Kubernetes and load balancer must be
	// up before any components can be installed.
	if err := installKubernetes(ctx, cfg, out); err != nil {
		return err
	}
	if err := installLoadBalancer(ctx, cfg, out); err != nil {
		return err
	}

	// Phase 2: Parallel component installation
	status(out, "Beginning Cluster Configuration")
	fmt.Fprintln(out, "Tasks will be executed in parallel.  Logs will be prefixed:")
	if cfg.Serving {
		fmt.Fprintln(out, "svr:  Serving, DNS and Networking")
	}
	if cfg.Eventing {
		fmt.Fprintln(out, "evt:  Eventing and Namespace")
	}
	fmt.Fprintln(out, "reg:  Local Registry")
	if cfg.Tekton {
		fmt.Fprintln(out, "tkt:  Tekton Pipelines")
	}
	if cfg.Keda {
		fmt.Fprintln(out, "keda: Keda")
	}
	fmt.Fprintln(out)

	g, gctx := errgroup.WithContext(ctx)

	// svr: serving -> dns -> networking (sequential within goroutine)
	if cfg.Serving {
		g.Go(func() error {
			w := newPrefixedWriter(out, "svr  ")
			defer w.Flush()
			if err := installServing(gctx, cfg, w); err != nil {
				return fmt.Errorf("serving: %w", err)
			}
			if err := configureDNS(gctx, cfg, w); err != nil {
				return fmt.Errorf("dns: %w", err)
			}
			return installNetworking(gctx, cfg, w)
		})
	}

	// evt: eventing -> namespace
	if cfg.Eventing {
		g.Go(func() error {
			w := newPrefixedWriter(out, "evt  ")
			defer w.Flush()
			if err := installEventing(gctx, cfg, w); err != nil {
				return fmt.Errorf("eventing: %w", err)
			}
			return configureNamespace(gctx, cfg, w)
		})
	}

	// reg: registry (always)
	g.Go(func() error {
		w := newPrefixedWriter(out, "reg  ")
		defer w.Flush()
		return installRegistry(gctx, cfg, w)
	})

	// tkt: tekton -> pac
	if cfg.Tekton {
		g.Go(func() error {
			w := newPrefixedWriter(out, "tkt  ")
			defer w.Flush()
			if err := installTekton(gctx, cfg, w); err != nil {
				return fmt.Errorf("tekton: %w", err)
			}
			return installPAC(gctx, cfg, w)
		})
	}

	// keda: keda -> keda_http_addon
	if cfg.Keda {
		g.Go(func() error {
			w := newPrefixedWriter(out, "keda ")
			defer w.Flush()
			if err := installKeda(gctx, cfg, w); err != nil {
				return fmt.Errorf("keda: %w", err)
			}
			return installKedaHTTPAddon(gctx, cfg, w)
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	// Phase 3: Magic DNS (requires all services to be up)
	if err := configureMagicDNS(ctx, cfg, out); err != nil {
		return err
	}

	printNextSteps(cfg, out)

	fmt.Fprintf(out, "\n%s\n\n", green("DONE"))
	return nil
}

// allocationCleanup runs after allocateCluster to report the outcome. On
// success it's a no-op; on failure it prints the error and either leaves
// the partial cluster in place (--no-cleanup) or tears it down.
func allocationCleanup(ctx context.Context, cfg ClusterConfig, out io.Writer, err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(out, "%s\n", red(fmt.Sprintf("Allocation failed: %v", err)))
	if cfg.NoCleanup {
		fmt.Fprintln(out, yellow("Cluster left in place for inspection (--no-cleanup). Clean up with: func cluster delete"))
		return
	}
	// Delete is best-effort: it emits yellow warnings inline and always
	// returns nil. The outer allocation error (`err`) is what callers see.
	_ = Delete(ctx, cfg, out)
	fmt.Fprintln(out)
	fmt.Fprintln(out, yellow("To inspect a failed cluster, retry with --no-cleanup:"))
	fmt.Fprintf(out, "  func cluster create --no-cleanup\n")
	fmt.Fprintln(out)
}

func printNextSteps(cfg ClusterConfig, out io.Writer) {
	fmt.Fprintf(out, `
Next Steps
----------

To use the new cluster, set the following environment variable:

  export KUBECONFIG=%[1]s
`, cfg.Kubeconfig())
}
