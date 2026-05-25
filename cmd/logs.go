package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/knative"
)

func NewLogsCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Stream logs from a deployed function",
		Long: `Stream logs from a deployed function

Streams logs for the function in the current directory or from the directory
specified with --path. Abstracts away the underlying service name and pod details.
`,
		Example: `
# Stream logs for the function in the current directory
{{rootCmdUse}} logs

# Stream logs for a function by name
{{rootCmdUse}} logs --name my-function

# Stream logs from a specific namespace
{{rootCmdUse}} logs --namespace my-namespace

# Stream logs with a specific time window
{{rootCmdUse}} logs --since 5m
`,
		SuggestFor:        []string{"log", "tail"},
		ValidArgsFunction: CompleteFunctionList,
		PreRunE:           bindEnv("name", "namespace", "path", "since", "verbose"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogs(cmd, newClient)
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
	cmd.Flags().StringP("since", "", "1m", "Return logs newer than a relative duration like 5s, 2m, or 3h ($FUNC_LOGS_SINCE)")
	addPathFlag(cmd)
	addVerboseFlag(cmd, cfg.Verbose)

	return cmd
}

func runLogs(cmd *cobra.Command, newClient ClientFactory) error {
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

	// Parse since duration
	var sinceTime *time.Time
	if cfg.Since != "" {
		duration, err := time.ParseDuration(cfg.Since)
		if err != nil {
			return fmt.Errorf("invalid duration format for --since: %w", err)
		}
		t := time.Now().Add(-duration)
		sinceTime = &t
	}

	// Create context that can be cancelled with Ctrl+C
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	// Handle interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Fprintln(os.Stderr, "\nStopping log stream...")
		cancel()
	}()

	fmt.Fprintf(os.Stderr, "Streaming logs for function '%s' in namespace '%s'...\n", f.Name, f.Namespace)
	fmt.Fprintf(os.Stderr, "Press Ctrl+C to stop.\n\n")

	err = knative.GetKServiceLogs(ctx, f.Namespace, f.Name, f.Image, sinceTime, os.Stdout)
	if err != nil && err != context.Canceled {
		return fmt.Errorf("failed to stream logs: %w", err)
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
	Verbose   bool
}

func newLogsConfig(cmd *cobra.Command) (cfg logsConfig, err error) {
	cfg = logsConfig{
		Name:      viper.GetString("name"),
		Namespace: viper.GetString("namespace"),
		Path:      viper.GetString("path"),
		Since:     viper.GetString("since"),
		Verbose:   viper.GetBool("verbose"),
	}

	if cfg.Name != "" && cmd.Flags().Changed("path") {
		// logically inconsistent to provide both a name and a path to source.
		err = ErrNameAndPathConflict
	}

	return
}
