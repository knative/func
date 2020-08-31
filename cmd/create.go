package cmd

import (
	"fmt"
	"path/filepath"

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
	createCmd.Flags().StringP("image", "i", "", "Optional full image name, in form [registry]/[namespace]/[name]:[tag] for example quay.io/myrepo/project.name:latest (overrides --repository) - $FAAS_IMAGE")
	createCmd.Flags().StringP("namespace", "n", "", "Override namespace into which the Function is deployed (on supported platforms).  Default is to use currently active underlying platform setting - $FAAS_NAMESPACE")
	createCmd.Flags().StringP("path", "p", cwd(), "Path to the new project directory - $FAAS_PATH")
	createCmd.Flags().StringP("repository", "r", "", "Repository for built images, ex 'docker.io/myuser' or just 'myuser'.  Optional if --image provided. - $FAAS_REPOSITORY")
	createCmd.Flags().StringP("runtime", "l", faas.DefaultRuntime, "Function runtime language/framework. - $FAAS_RUNTIME")
	createCmd.Flags().StringP("templates", "", filepath.Join(configPath(), "faas", "templates"), "Extensible templates path. - $FAAS_TEMPLATES")
	createCmd.Flags().StringP("trigger", "t", faas.DefaultTrigger, "Function trigger (ex: 'http','events') - $FAAS_TRIGGER")
	createCmd.Flags().BoolP("yes", "y", false, "When in interactive mode (attached to a TTY) skip prompts. - $FAAS_YES")

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
	Use:        "create <name> [options]",
	Short:      "Create a new Function, including initialization of local files and deployment.",
	SuggestFor: []string{"cerate", "new"},
	PreRunE:    bindEnv("image", "namespace", "path", "repository", "runtime", "templates", "trigger", "yes"),
	RunE:       runCreate,
}

func runCreate(cmd *cobra.Command, args []string) (err error) {
	config := newCreateConfig(args).Prompt()

	function := faas.Function{
		Name:    config.Name,
		Root:    config.initConfig.Path,
		Runtime: config.initConfig.Runtime,
		Trigger: config.Trigger,
		Image:   config.Image,
	}

	builder := buildpacks.NewBuilder()
	builder.Verbose = config.initConfig.Verbose

	pusher := docker.NewPusher()
	pusher.Verbose = config.initConfig.Verbose

	deployer := knative.NewDeployer()
	deployer.Verbose = config.initConfig.Verbose

	listener := progress.New()
	listener.Verbose = config.initConfig.Verbose

	client := faas.New(
		faas.WithVerbose(config.initConfig.Verbose),
		faas.WithTemplates(config.Templates),
		faas.WithRepository(config.Repository), // for deriving image name when --image not provided explicitly.
		faas.WithBuilder(builder),
		faas.WithPusher(pusher),
		faas.WithDeployer(deployer),
		faas.WithProgressListener(listener))

	return client.Create(function)
}

type createConfig struct {
	initConfig
	buildConfig
	deployConfig
	// Note that ambiguous references set to assume .initConfig
}

func newCreateConfig(args []string) createConfig {
	return createConfig{
		initConfig:   newInitConfig(args),
		buildConfig:  newBuildConfig(),
		deployConfig: newDeployConfig(),
	}
}

// Prompt the user with value of config members, allowing for interaractive changes.
// Skipped if not in an interactive terminal (non-TTY), or if --yes (agree to
// all prompts) was explicitly set.
func (c createConfig) Prompt() createConfig {
	if !interactiveTerminal() || c.initConfig.Yes {
		return c
	}
	return createConfig{
		initConfig: initConfig{
			Path:    prompt.ForString("Path to project directory", c.initConfig.Path),
			Name:    prompt.ForString("Function project name", deriveName(c.Name, c.initConfig.Path), prompt.WithRequired(true)),
			Verbose: prompt.ForBool("Verbose logging", c.initConfig.Verbose),
			Runtime: prompt.ForString("Runtime of source", c.Runtime),
			Trigger: prompt.ForString("Function Trigger", c.Trigger),
			// Templates intentiopnally omitted from prompt for being an edge case.
		},
		buildConfig: buildConfig{
			Repository: prompt.ForString("Repository for Function images", c.buildConfig.Repository),
		},
		deployConfig: deployConfig{
			Namespace: prompt.ForString("Override default deploy namespace", c.Namespace),
		},
	}
}
