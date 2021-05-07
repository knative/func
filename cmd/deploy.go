package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/containers/image/v5/pkg/docker/config"
	containersTypes "github.com/containers/image/v5/types"
	"github.com/ory/viper"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	fn "github.com/boson-project/func"
	"github.com/boson-project/func/buildpacks"
	"github.com/boson-project/func/docker"
	"github.com/boson-project/func/knative"
	"github.com/boson-project/func/progress"
	"github.com/boson-project/func/prompt"
)

func init() {
	root.AddCommand(deployCmd)
	deployCmd.Flags().BoolP("confirm", "c", false, "Prompt to confirm all configuration options (Env: $FUNC_CONFIRM)")
	deployCmd.Flags().StringArrayP("env", "e", []string{}, "Environment variable to set in the form NAME=VALUE. " +
		"You may provide this flag multiple times for setting multiple environment variables. " +
		"To unset, specify the environment variable name followed by a \"-\" (e.g., NAME-).")
	deployCmd.Flags().StringP("image", "i", "", "Full image name in the form [registry]/[namespace]/[name]:[tag] (optional). This option takes precedence over --registry (Env: $FUNC_IMAGE")
	deployCmd.Flags().StringP("namespace", "n", "", "Namespace of the function to undeploy. By default, the namespace in func.yaml is used or the actual active namespace if not set in the configuration. (Env: $FUNC_NAMESPACE)")
	deployCmd.Flags().StringP("path", "p", cwd(), "Path to the project directory (Env: $FUNC_PATH)")
	deployCmd.Flags().StringP("registry", "r", "", "Registry + namespace part of the image to build, ex 'quay.io/myuser'.  The full image name is automatically determined based on the local directory name. If not provided the registry will be taken from func.yaml (Env: $FUNC_REGISTRY)")
	deployCmd.Flags().BoolP("build", "b", true, "Build the image before deploying (Env: $FUNC_BUILD)")
}

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy a function",
	Long: `Deploy a function

Builds a container image for the function and deploys it to the connected Knative enabled cluster. 
The function is picked up from the project in the current directory or from the path provided
with --path.
If not already configured, either --registry or --image has to be provided and is then stored 
in the configuration file.

If the function is already deployed, it is updated with a new container image
that is pushed to an image registry, and finally the function's Knative service is updated.
`,
	Example: `
# Build and deploy the function from the current directory's project. The image will be
# pushed to "quay.io/myuser/<function name>" and deployed as Knative service with the 
# same name as the function to the currently connected cluster.
kn func deploy --registry quay.io/myuser

# Same as above but using a full image name, that will create a Knative service "myfunc" in 
# the namespace "myns"
kn func deploy --image quay.io/myuser/myfunc -n myns
`,
	SuggestFor: []string{"delpoy", "deplyo"},
	PreRunE:    bindEnv("image", "namespace", "path", "registry", "confirm", "build"),
	RunE:       runDeploy,
}

func runDeploy(cmd *cobra.Command, _ []string) (err error) {

	config := newDeployConfig(cmd).Prompt()

	function, err := functionWithOverrides(config.Path, functionOverrides{Namespace: config.Namespace, Image: config.Image})
	if err != nil {
		return
	}

	function.Env = mergeEnvMaps(function.Env, config.Env)

	// Check if the Function has been initialized
	if !function.Initialized() {
		return fmt.Errorf("the given path '%v' does not contain an initialized function. Please create one at this path before deploying", config.Path)
	}

	// If the Function does not yet have an image name and one was not provided on the command line
	if function.Image == "" {
		//  AND a --registry was not provided, then we need to
		// prompt for a registry from which we can derive an image name.
		if config.Registry == "" {
			fmt.Print("A registry for Function images is required. For example, 'docker.io/tigerteam'.\n\n")
			config.Registry = prompt.ForString("Registry for Function images", "")
			if config.Registry == "" {
				return fmt.Errorf("unable to determine function image name")
			}
		}

		// We have the registry, so let's use it to derive the Function image name
		config.Image = deriveImage(config.Image, config.Registry, config.Path)
		function.Image = config.Image
	}

	// All set, let's write changes in the config to the disk
	err = function.WriteConfig()
	if err != nil {
		return
	}

	builder := buildpacks.NewBuilder()
	builder.Verbose = config.Verbose

	pusher, err := docker.NewPusher(docker.WithCredentialsProvider(credentialsProvider))
	if err != nil {
		return err
	}
	pusher.Verbose = config.Verbose

	ns := config.Namespace
	if ns == "" {
		ns = function.Namespace
	}

	deployer, err := knative.NewDeployer(ns)
	if err != nil {
		return
	}

	listener := progress.New()
	defer listener.Done()

	deployer.Verbose = config.Verbose
	listener.Verbose = config.Verbose

	context := cmd.Context()
	go func() {
		<-context.Done()
		listener.Done()
	}()

	client := fn.New(
		fn.WithVerbose(config.Verbose),
		fn.WithRegistry(config.Registry), // for deriving image name when --image not provided explicitly.
		fn.WithBuilder(builder),
		fn.WithPusher(pusher),
		fn.WithDeployer(deployer),
		fn.WithProgressListener(listener))

	if config.Build {
		if err := client.Build(context, config.Path); err != nil {
			return err
		}
	}

	return client.Deploy(context, config.Path)

	// NOTE: Namespace is optional, default is that used by k8s client
	// (for example kubectl usually uses ~/.kube/config)
}

func credentialsProvider(ctx context.Context, registry string) (docker.Credentials, error) {

	result := docker.Credentials{}
	credentials, err := config.GetCredentials(nil, registry)
	if err != nil {
		return result, errors.Wrap(err, "failed to get credentials")
	}

	if credentials != (containersTypes.DockerAuthConfig{}) {
		result.Username, result.Password = credentials.Username, credentials.Password
		return result, nil
	}

	fmt.Print("Username: ")
	username, err := getUserName(ctx)
	if err != nil {
		return result, err
	}

	fmt.Print("Password: ")
	bytePassword, err := getPassword(ctx)
	if err != nil {
		return result, err
	}
	password := string(bytePassword)

	result.Username, result.Password = username, password

	return result, nil
}

func getPassword(ctx context.Context) ([]byte, error) {
	ch := make(chan struct {
		p []byte
		e error
	})

	go func() {
		pass, err := term.ReadPassword(0)
		ch <- struct {
			p []byte
			e error
		}{p: pass, e: err}
	}()

	select {
	case res := <-ch:
		return res.p, res.e
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func getUserName(ctx context.Context) (string, error) {
	ch := make(chan struct {
		u string
		e error
	})
	go func() {
		reader := bufio.NewReader(os.Stdin)
		username, err := reader.ReadString('\n')
		if err != nil {
			ch <- struct {
				u string
				e error
			}{u: "", e: err}
		}
		ch <- struct {
			u string
			e error
		}{u: strings.TrimRight(username, "\n"), e: nil}
	}()

	select {
	case res := <-ch:
		return res.u, res.e
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

type deployConfig struct {
	buildConfig

	// Namespace override for the deployed function.  If provided, the
	// underlying platform will be instructed to deploy the function to the given
	// namespace (if such a setting is applicable; such as for Kubernetes
	// clusters).  If not provided, the currently configured namespace will be
	// used.  For instance, that which would be used by default by `kubectl`
	// (~/.kube/config) in the case of Kubernetes.
	Namespace string

	// Path of the Function implementation on local disk. Defaults to current
	// working directory of the process.
	Path string

	// Verbose logging.
	Verbose bool

	// Confirm: confirm values arrived upon from environment plus flags plus defaults,
	// with interactive prompting (only applicable when attached to a TTY).
	Confirm bool

	// Build the associated Function before deploying.
	Build bool

	Env map[string]string
}

// newDeployConfig creates a buildConfig populated from command flags and
// environment variables; in that precedence.
func newDeployConfig(cmd *cobra.Command) deployConfig {
	return deployConfig{
		buildConfig: newBuildConfig(),
		Namespace:   viper.GetString("namespace"),
		Path:        viper.GetString("path"),
		Verbose:     viper.GetBool("verbose"), // defined on root
		Confirm:     viper.GetBool("confirm"),
		Build:       viper.GetBool("build"),
		Env:         envFromCmd(cmd),
	}
}

// Prompt the user with value of config members, allowing for interaractive changes.
// Skipped if not in an interactive terminal (non-TTY), or if --yes (agree to
// all prompts) was explicitly set.
func (c deployConfig) Prompt() deployConfig {
	if !interactiveTerminal() || !c.Confirm {
		return c
	}
	dc := deployConfig{
		buildConfig: buildConfig{
			Registry: prompt.ForString("Registry for Function images", c.buildConfig.Registry),
		},
		Namespace: prompt.ForString("Namespace", c.Namespace),
		Path:      prompt.ForString("Project path", c.Path),
		Verbose:   c.Verbose,
	}

	dc.Image = deriveImage(dc.Image, dc.Registry, dc.Path)

	return dc
}
