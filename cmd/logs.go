package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/knative"
)

// logGatherer writes the logs of a deployed function.
type logGatherer func(context.Context, knative.LogsOptions, io.Writer) error

func NewLogsCmd(newClient ClientFactory) *cobra.Command {
	return newLogsCmd(newClient, knative.GetKServiceLogs)
}

// newLogsCmd constructs the command with an explicit log gatherer, allowing
// tests to exercise the command without a cluster.
func newLogsCmd(newClient ClientFactory, gather logGatherer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Print or stream logs from a deployed function",
		Long: `Print or stream logs from a deployed function

Prints the logs of the function in the current directory or of the directory
specified with --path and exits. Use --follow to stream logs until interrupted.
Abstracts away the underlying service name and pod details.

When more than one pod is serving the function, each line is prefixed with the
pod it came from. A function which has scaled to zero has no pods, and thus no
logs to print.

Only functions deployed with the default Knative deployer are currently
supported.
`,
		Example: `
# Print the logs of the function in the current directory and exit
{{rootCmdUse}} logs

# Stream logs until interrupted
{{rootCmdUse}} logs -f

# Print logs for a function by name
{{rootCmdUse}} logs --name my-function

# Print logs from a specific namespace
{{rootCmdUse}} logs --namespace my-namespace

# Print logs of a specific time window
{{rootCmdUse}} logs --since 5m

# Print the last 20 log lines per pod
{{rootCmdUse}} logs --tail 20

# Stream logs, starting with those of the last 5 minutes
{{rootCmdUse}} logs -f --since 5m
`,
		SuggestFor:        []string{"log", "tail"},
		ValidArgsFunction: CompleteFunctionList,
		PreRunE:           bindEnv("name", "namespace", "path", "since", "tail", "follow", "verbose"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogs(cmd, newClient, gather)
		},
	}

	// Config
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	// Flags
	cmd.Flags().StringP("name", "", "", "Name of the function to get logs from ($FUNC_NAME)")
	cmd.Flags().StringP("namespace", "n", defaultNamespace(fn.Function{}, false), "The namespace of the function ($FUNC_NAMESPACE)")
	cmd.Flags().StringP("since", "", "", "Return logs newer than a relative duration like 5s, 2m, or 3h. Defaults to all available logs, or to the last minute when following without --tail ($FUNC_SINCE)")
	cmd.Flags().Int64P("tail", "", -1, "Number of most recent log lines to return per pod. Unlimited if negative ($FUNC_TAIL)")
	cmd.Flags().BoolP("follow", "f", false, "Stream logs until interrupted rather than printing and exiting ($FUNC_FOLLOW)")
	addPathFlag(cmd)
	addVerboseFlag(cmd, cfg.Verbose)

	return cmd
}

func runLogs(cmd *cobra.Command, newClient ClientFactory, gather logGatherer) error {
	cfg, err := newLogsConfig(cmd)
	if err != nil {
		return err
	}

	client, done := newClient(ClientConfig{Verbose: cfg.Verbose})
	defer done()

	// Get function details and deployer type
	var f fn.Function
	var deployer string
	if cfg.Name != "" {
		// Get function by name
		instance, err := client.Describe(cmd.Context(), cfg.Name, cfg.Namespace, fn.Function{})
		if err != nil {
			return fmt.Errorf("failed to get function details: %w", err)
		}
		f.Name = instance.Name
		f.Namespace = instance.Namespace
		f.Image = instance.Image
		deployer = instance.Deployer
	} else {
		// Load function from path
		f, err = fn.NewFunction(cfg.Path)
		if err != nil {
			return err
		}
		if !f.Initialized() {
			return NewErrNotInitializedFromPath(f.Root, "logs")
		}

		// Get deployed function details to ensure it exists
		instance, err := client.Describe(cmd.Context(), "", "", f)
		if err != nil {
			return fmt.Errorf("function not deployed or not found: %w", err)
		}
		f.Name = instance.Name
		f.Namespace = instance.Namespace
		f.Image = instance.Image
		deployer = instance.Deployer
	}

	// Guard: the knative log streamer uses a serving.knative.dev/service
	// label selector that only matches pods created by the Knative deployer.
	// For other deployer types, return a clear error rather than silently
	// producing no output.
	if deployer != "" && deployer != "knative" {
		return fmt.Errorf("'func logs' is not yet supported for the %q deployer.\n"+
			"Currently only functions deployed with the default Knative deployer are supported.\n"+
			"You can use 'kubectl logs' directly to view logs for %s-deployed functions", deployer, deployer)
	}

	// Limit the lines returned per pod if requested
	var tailLines *int64
	if cfg.Tail >= 0 {
		tailLines = &cfg.Tail
	}

	// Bound how far back logs are read.  A snapshot defaults to everything
	// available, as does kubectl, because a default window would silently
	// truncate --tail and hide the logs of a function which has been idle.
	// Following, however, keeps its historical one minute of context, unless
	// --tail already bounds the output.
	since := cfg.Since
	if since == "" && cfg.Follow && tailLines == nil {
		since = "1m"
	}
	var sinceTime *time.Time
	if since != "" {
		duration, err := time.ParseDuration(since)
		if err != nil {
			return fmt.Errorf("invalid duration format for --since: %w", err)
		}
		t := time.Now().Add(-duration)
		sinceTime = &t
	}

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	// Following is interactive and unbounded: announce it on stderr, keeping
	// stdout log content only, and stop on interrupt.
	if cfg.Follow {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(sigChan)
		go func() {
			select {
			case <-sigChan:
				fmt.Fprintln(cmd.ErrOrStderr(), "\nStopping log stream...")
				cancel()
			case <-ctx.Done():
			}
		}()

		fmt.Fprintf(cmd.ErrOrStderr(), "Streaming logs for function '%s' in namespace '%s'...\n", f.Name, f.Namespace)
		fmt.Fprintf(cmd.ErrOrStderr(), "Press Ctrl+C to stop.\n\n")
	}

	// NOTE: the image of the described instance is deliberately not used to
	// filter pods.  It is that of the latest revision, so filtering on it
	// would drop the logs of the pods actually serving traffic during a
	// rollout.  All pods of the function's service are of interest here.
	err = gather(ctx, knative.LogsOptions{
		Name:      f.Name,
		Namespace: f.Namespace,
		Since:     sinceTime,
		TailLines: tailLines,
		Follow:    cfg.Follow,
	}, cmd.OutOrStdout())

	// A function which has scaled to zero has no pods, and thus no logs.  This
	// is not a failure, but it is reported such that an empty stdout is not
	// mistaken for a function which is running silently.
	if errors.Is(err, k8s.ErrNoMatchingPods) {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"No running or recently terminated pods found for function '%s' in namespace '%s'. "+
				"It may have scaled to zero, in which case there are no logs to print.\n", f.Name, f.Namespace)
		return nil
	}

	// Pods can disappear while their logs are being read, e.g. when a revision
	// is scaled down.  Report it, but keep the logs which were gathered and
	// the successful exit code.
	var partial *k8s.PartialLogsError
	if errors.As(err, &partial) {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %v\n", partial)
		return nil
	}

	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("failed to get logs: %w", err)
	}

	return nil
}

// CLI Configuration (parameters)
// ------------------------------

type logsConfig struct {
	Name      string
	Namespace string
	Path      string
	Since     string
	Tail      int64
	Follow    bool
	Verbose   bool
}

func newLogsConfig(cmd *cobra.Command) (cfg logsConfig, err error) {
	cfg = logsConfig{
		Name:      viper.GetString("name"),
		Namespace: viper.GetString("namespace"),
		Path:      viper.GetString("path"),
		Since:     viper.GetString("since"),
		Tail:      viper.GetInt64("tail"),
		Follow:    viper.GetBool("follow"),
		Verbose:   viper.GetBool("verbose"),
	}

	if cfg.Name != "" && cmd.Flags().Changed("path") {
		// logically inconsistent to provide both a name and a path to source.
		err = ErrNameAndPathConflict
	}

	return
}
