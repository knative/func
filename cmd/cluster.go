package cmd

import (
	"fmt"
	"os"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"knative.dev/func/pkg/cluster"
)

// NewClusterCmd creates the parent command for cluster management.
//
// The command is marked Hidden and gated behind the FUNC_ENABLE_CLUSTER
// environment variable: invoking any subcommand without it set returns a
// non-zero exit and a helpful message. The command still appears in
// explicit `func cluster --help`, but not in the top-level listing.
func NewClusterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cluster",
		Aliases: []string{"clusters"},
		Short:   "Manage local clusters (experimental)",
		Hidden:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if os.Getenv("FUNC_ENABLE_CLUSTER") == "" {
				return fmt.Errorf(
					"'%s cluster' is an experimental feature and is not enabled by default.\n"+
						"Set FUNC_ENABLE_CLUSTER to a non-empty value to enable it, e.g.:\n"+
						"  export FUNC_ENABLE_CLUSTER=1",
					cmd.Root().Name())
			}
			return nil
		},
		Long: `
NAME
	{{rootCmdUse}} cluster - Manage local development clusters (experimental)

SYNOPSIS
	{{rootCmdUse}} cluster <create|delete|list> [flags]

DESCRIPTION
	Manages local development clusters with Knative, Tekton, and other
	components pre-installed, alongside a local container registry for
	function image builds.

	This is an experimental feature; set FUNC_ENABLE_CLUSTER to a
	non-empty value to enable it.

	See '{{rootCmdUse}} cluster create --help' for the full list of
	configurable components and flags.

EXAMPLES
	o Create a default development cluster
	  $ {{rootCmdUse}} cluster create

	o Create a minimal cluster (just Kubernetes + registry)
	  $ {{rootCmdUse}} cluster create --serving=false --eventing=false

	o List existing clusters
	  $ {{rootCmdUse}} cluster list

	o Remove the default cluster
	  $ {{rootCmdUse}} cluster delete

`,
	}
	cmd.AddCommand(NewClusterCreateCmd())
	cmd.AddCommand(NewClusterDeleteCmd())
	cmd.AddCommand(NewClusterListCmd())
	return cmd
}

// NewClusterCreateCmd creates the 'func cluster create' command.
func NewClusterCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a local development cluster",
		Long: `
NAME
	{{rootCmdUse}} cluster create - Create a local development cluster

SYNOPSIS
	{{rootCmdUse}} cluster create [-n|--name] [--domain] [--serving] [--eventing]
	             [--tekton] [--keda] [--container-engine]
	             [--namespace] [--pac-host]
	             [--skip-binaries] [--skip-registry-config] [--no-cleanup]
	             [--retries]

DESCRIPTION
	Creates a local development cluster preconfigured to run Functions,
	along with a local container registry and the components needed for
	the function runtime (Serving, Eventing, Tekton, etc).

	The generated kubeconfig is written under the func config directory and
	does not alter the system's existing kubernetes configuration. To use the
	cluster, export the KUBECONFIG path shown on successful creation, e.g.:

	  export KUBECONFIG=~/.config/func/clusters/func.local/kubeconfig.yaml

	Installed components are controlled by the --serving, --eventing,
	--tekton, and --keda flags. Required binaries are downloaded into the
	func config directory on first use unless --skip-binaries is set.

EXAMPLES
	o Create a default development cluster
	  $ {{rootCmdUse}} cluster create

	o Create a minimal cluster (just Kubernetes + registry)
	  $ {{rootCmdUse}} cluster create --serving=false --eventing=false

	o Create a cluster with a custom name and domain
	  $ {{rootCmdUse}} cluster create --name myproject --domain example.local

	o Preserve a failed cluster for inspection
	  $ {{rootCmdUse}} cluster create --no-cleanup
`,

		PreRunE: bindEnv("name", "retries", "serving", "eventing", "tekton",
			"keda", "domain", "container-engine", "namespace",
			"pac-host", "skip-binaries",
			"skip-registry-config", "no-cleanup"),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := newClusterCreateConfig()
			if err := cluster.Create(cmd.Context(), cfg, cmd.OutOrStderr()); err != nil {
				return err
			}
			// Scriptable output: on success stdout receives only the
			// kubeconfig path, so callers can do e.g.
			//   export KUBECONFIG=$(func cluster create)
			fmt.Fprintln(cmd.OutOrStdout(), cfg.Kubeconfig())
			return nil
		},
	}

	cmd.Flags().StringP("name", "n", "func",
		"Cluster name ($FUNC_CLUSTER_NAME)")
	cmd.Flags().Int("retries", 1,
		"Max cluster allocation attempts ($FUNC_CLUSTER_RETRIES)")
	cmd.Flags().Bool("serving", true,
		"Install Knative Serving ($FUNC_CLUSTER_SERVING)")
	cmd.Flags().Bool("eventing", true,
		"Install Knative Eventing ($FUNC_CLUSTER_EVENTING)")
	cmd.Flags().Bool("tekton", false,
		"Install Tekton + Pipelines-as-Code for in-cluster (remote) builds ($FUNC_CLUSTER_TEKTON)")
	cmd.Flags().Bool("keda", false,
		"Install KEDA ($FUNC_CLUSTER_KEDA)")
	cmd.Flags().String("domain", "localtest.me",
		"DNS domain for services ($FUNC_CLUSTER_DOMAIN)")
	cmd.Flags().String("container-engine", "",
		"Container engine: docker or podman; auto-detected if unset, preferring docker when both are installed ($FUNC_CONTAINER_ENGINE)")
	cmd.Flags().String("namespace", "default",
		"Kubernetes namespace for RBAC bindings ($FUNC_NAMESPACE)")
	cmd.Flags().String("pac-host", "pac-ctr.localtest.me",
		"PAC controller hostname ($FUNC_INT_PAC_HOST)")
	cmd.Flags().Bool("skip-binaries", false,
		"Skip automatic binary downloads ($FUNC_SKIP_BINARIES)")
	cmd.Flags().Bool("skip-registry-config", false,
		"Skip host registry configuration ($FUNC_SKIP_REGISTRY_CONFIG)")
	cmd.Flags().Bool("no-cleanup", false,
		"Don't delete cluster on failure ($FUNC_NO_CLEANUP)")

	return cmd
}

func newClusterCreateConfig() cluster.ClusterConfig {
	return cluster.ClusterConfig{
		Name:                    viper.GetString("name"),
		Domain:                  viper.GetString("domain"),
		Serving:                 viper.GetBool("serving"),
		Eventing:                viper.GetBool("eventing"),
		Tekton:                  viper.GetBool("tekton"),
		Keda:                    viper.GetBool("keda"),
		Retries:                 viper.GetInt("retries"),
		Namespace:               viper.GetString("namespace"),
		PacHost:                 viper.GetString("pac-host"),
		SkipBinaries:            viper.GetBool("skip-binaries"),
		SkipRegistryConfig:      viper.GetBool("skip-registry-config"),
		NoCleanup:               viper.GetBool("no-cleanup"),
		ContainerEngineOverride: viper.GetString("container-engine"),
		KubectlOverride:         os.Getenv("FUNC_TEST_KUBECTL"),        // override binary path
		KindOverride:            os.Getenv("FUNC_TEST_KIND"),           // override binary path
		GitHubActions:           os.Getenv("GITHUB_ACTIONS") == "true", // detect CI environments
	}
}

// NewClusterDeleteCmd creates the 'func cluster delete' command.
func NewClusterDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [name]",
		Short: "Delete a local development cluster",
		Long: `
NAME
	{{rootCmdUse}} cluster delete - Delete a local development cluster

SYNOPSIS
	{{rootCmdUse}} cluster delete [name] [--container-engine]
	             [--skip-registry-config]

DESCRIPTION
	Deletes a local development cluster and its associated registry
	container. If no name is given, the default cluster "func" is deleted.

	When multiple func-managed clusters exist, specify which one by name.
	Use '{{rootCmdUse}} cluster list' to see existing clusters.

	Pass --skip-registry-config to mirror a create invocation that
	used the same flag; otherwise delete attempts to remove host
	registry-trust entries it never added (harmless but noisy, and may
	prompt for sudo).

EXAMPLES
	o Delete the default "func" cluster
	  $ {{rootCmdUse}} cluster delete

	o Delete a named cluster
	  $ {{rootCmdUse}} cluster delete myproject

`,

		PreRunE: bindEnv("container-engine", "skip-registry-config"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runClusterDelete(cmd, args)
		},
	}

	cmd.Flags().String("container-engine", "",
		"Container engine: docker or podman; auto-detected if unset, preferring docker when both are installed ($FUNC_CONTAINER_ENGINE)")
	cmd.Flags().Bool("skip-registry-config", false,
		"Skip host registry configuration revert ($FUNC_SKIP_REGISTRY_CONFIG)")

	return cmd
}

func newClusterDeleteConfig() cluster.ClusterConfig {
	return cluster.ClusterConfig{
		Name:                    "func",
		ContainerEngineOverride: viper.GetString("container-engine"),
		SkipRegistryConfig:      viper.GetBool("skip-registry-config"),
		KubectlOverride:         os.Getenv("FUNC_TEST_KUBECTL"),
		KindOverride:            os.Getenv("FUNC_TEST_KIND"),
		GitHubActions:           os.Getenv("GITHUB_ACTIONS") == "true",
	}
}

func runClusterDelete(cmd *cobra.Command, args []string) error {
	cfg := newClusterDeleteConfig()
	if len(args) > 0 {
		cfg.Name = args[0]
	}

	clusters := cluster.List()
	for _, c := range clusters {
		if c == cfg.Name {
			return cluster.Delete(cmd.Context(), cfg, cmd.OutOrStderr())
		}
	}

	if len(clusters) == 0 {
		return fmt.Errorf("no clusters exist; use 'func cluster create' to create one")
	}
	return fmt.Errorf("cluster %q not found; existing clusters: %v\nUse 'func cluster create' to create one", cfg.Name, clusters)
}

// NewClusterListCmd creates the 'func cluster list' command.
func NewClusterListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List local development clusters",
		Long: `
NAME
	{{rootCmdUse}} cluster list - List local development clusters

SYNOPSIS
	{{rootCmdUse}} cluster list

DESCRIPTION
	Lists local development clusters managed by func. Output is the bare
	cluster name, one per line.

EXAMPLES
	o List existing clusters
	  $ {{rootCmdUse}} cluster list

`,
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, name := range cluster.List() {
				fmt.Fprintln(cmd.OutOrStdout(), name)
			}
			return nil
		},
	}
}
