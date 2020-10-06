package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/buildpacks"
	"github.com/boson-project/faas/docker"
	"github.com/boson-project/faas/knative"
	"github.com/boson-project/faas/progress"
	"github.com/boson-project/faas/prompt"
)

func init() {
	root.AddCommand(createCmd)
	createCmd.Flags().BoolP("confirm", "c", false, "Prompt to confirm all configuration options - $FAAS_CONFIRM")
	createCmd.Flags().StringP("image", "i", "", "Optional full image name, in form [registry]/[namespace]/[name]:[tag] for example quay.io/myrepo/project.name:latest (overrides --registry) - $FAAS_IMAGE")
	createCmd.Flags().StringP("namespace", "n", "", "Override namespace into which the Function is deployed (on supported platforms).  Default is to use currently active underlying platform setting - $FAAS_NAMESPACE")
	createCmd.Flags().StringP("registry", "r", "", "Registry for built images, ex 'docker.io/myuser' or just 'myuser'.  Optional if --image provided. - $FAAS_REGISTRY")
	createCmd.Flags().StringP("runtime", "l", faas.DefaultRuntime, "Function runtime language/framework. Default runtime is 'go'. Available runtimes: 'node', 'quarkus' and 'go'. - $FAAS_RUNTIME")
	createCmd.Flags().StringP("templates", "", filepath.Join(configPath(), "templates"), "Extensible templates path. - $FAAS_TEMPLATES")
	createCmd.Flags().StringP("trigger", "t", faas.DefaultTrigger, "Function trigger. Default trigger is 'http'. Available triggers: 'http' and 'events' - $FAAS_TRIGGER")

	var err error
	err = createCmd.RegisterFlagCompletionFunc("image", CompleteRegistryList)
	if err != nil {
		fmt.Println("Error while calling RegisterFlagCompletionFunc: ", err)
	}
	err = createCmd.RegisterFlagCompletionFunc("runtime", CompleteRuntimeList)
	if err != nil {
		fmt.Println("Error while calling RegisterFlagCompletionFunc: ", err)
	}
}

var createCmd = &cobra.Command{
	Use:   "create <path>",
	Short: "Create a new Function, including initialization of local files and deployment",
	Long: `Create a new Function, including initialization of local files and deployment

Creates a new Function project at <path>. If <path> does not exist, it is
created. The Function name is the name of the leaf directory at <path>. After
creating the project, a container image is created and is deployed. This
command wraps "init", "build" and "deploy" all up into one command.

The runtime, trigger, image name, image registry, and namespace may all be
specified as flags on the command line, and will subsequently be the default
values when an image is built or a Function is deployed. If the image name and
image registry are both unspecified, the user will be prompted for an image
registry, and the image name can be inferred from that plus the function
name. The function name, namespace and image name are all persisted in the
project configuration file faas.yaml.
`,
	SuggestFor: []string{"cerate", "new"},
	PreRunE:    bindEnv("image", "namespace", "registry", "runtime", "templates", "trigger", "confirm"),
	RunE:       runCreate,
}

func runCreate(cmd *cobra.Command, args []string) (err error) {
	config := newCreateConfig(args).Prompt()

	function := faas.Function{
		Name:    config.initConfig.Name,
		Root:    config.initConfig.Path,
		Runtime: config.initConfig.Runtime,
		Trigger: config.Trigger,
		Image:   config.Image,
	}

	if function.Image == "" && config.Registry == "" {
		fmt.Print("A registry for Function images is required. For example, 'docker.io/tigerteam'.\n\n")
		config.Registry = prompt.ForString("Registry for Function images", "")
		if config.Registry == "" {
			return fmt.Errorf("Unable to determine Function image name")
		}
	}

	// Defined in root command
	verbose := viper.GetBool("verbose")

	builder := buildpacks.NewBuilder()
	builder.Verbose = verbose

	pusher := docker.NewPusher()
	pusher.Verbose = verbose

	deployer, err := knative.NewDeployer(config.Namespace)
	if err != nil {
		return
	}
	deployer.Verbose = verbose

	listener := progress.New()
	listener.Verbose = verbose

	client := faas.New(
		faas.WithVerbose(verbose),
		faas.WithTemplates(config.Templates),
		faas.WithRegistry(config.Registry), // for deriving image name when --image not provided explicitly.
		faas.WithBuilder(builder),
		faas.WithPusher(pusher),
		faas.WithDeployer(deployer),
		faas.WithProgressListener(listener))

	return client.Create(function)
}

type createConfig struct {
	initConfig
	deployConfig
	// Note that ambiguous references set to assume .initConfig
}

func newCreateConfig(args []string) createConfig {
	return createConfig{
		initConfig:   newInitConfig(args),
		deployConfig: newDeployConfig(),
	}
}

// Prompt the user with value of config members, allowing for interactive changes.
// Skipped if not in an interactive terminal (non-TTY), or if --confirm (agree to
// all prompts) was not explicitly set.
func (c createConfig) Prompt() createConfig {
	if !interactiveTerminal() || !c.initConfig.Confirm {
		// Just print the basics if not confirming
		fmt.Printf("Project path: %v\n", c.initConfig.Path)
		fmt.Printf("Function name: %v\n", c.initConfig.Name)
		fmt.Printf("Runtime: %v\n", c.Runtime)
		fmt.Printf("Trigger: %v\n", c.Trigger)
		return c
	}

	derivedName, derivedPath := deriveNameAndAbsolutePathFromPath(prompt.ForString("Project path", c.initConfig.Path, prompt.WithRequired(true)))
	return createConfig{
		initConfig: initConfig{
			Name:    derivedName,
			Path:    derivedPath,
			Runtime: prompt.ForString("Runtime", c.Runtime),
			Trigger: prompt.ForString("Trigger", c.Trigger),
			// Templates intentionally omitted from prompt for being an edge case.
		},
		deployConfig: deployConfig{
			buildConfig: buildConfig{
				Registry: prompt.ForString("Registry for Function images", c.buildConfig.Registry),
			},
			Namespace: prompt.ForString("Override default deploy namespace", c.Namespace),
		},
	}
}
