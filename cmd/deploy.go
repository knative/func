package cmd

import (
	"os"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/docker"
	"github.com/boson-project/faas/knative"
	"github.com/ory/viper"
	"github.com/spf13/cobra"
)

var deployCmd = &cobra.Command{
	Use:        "deploy",
	Short:      "Deploy an existing Function project to a cluster",
	SuggestFor: []string{"delpoy", "deplyo"},
	RunE:       deployImage,
	PreRunE: func(cmd *cobra.Command, args []string) (err error) {
		flags := []string{"build", "namespace", "path", "tag"}
		for _, f := range flags {
			err = viper.BindPFlag(f, cmd.Flags().Lookup(f))
			if err != nil {
				return err
			}

		}
		return
	},
}

func init() {
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	root.AddCommand(deployCmd)
	deployCmd.Flags().BoolP("build", "b", false, "Build the image prior to deployment")
	deployCmd.Flags().BoolP("expose", "e", true, "Create a publicly accessible route to the Function")
	deployCmd.Flags().StringP("namespace", "s", "default", "Cluster namespace to deploy the Function in")
	deployCmd.Flags().StringP("path", "p", cwd, "Path to the function project directory")
	deployCmd.Flags().StringP("tag", "t", "", "Specify an image tag, for example quay.io/myrepo/project.name:latest")
}

func deployImage(cmd *cobra.Command, args []string) (err error) {
	var config = buildConfig{
		Verbose: viper.GetBool("verbose"),
		Path:    viper.GetString("path"),
		Tag:     viper.GetString("tag"),
		Push:    true,
	}

	f, err := FunctionConfigForBuild(config)
	if err != nil {
		return err
	}

	if viper.GetBool("build") {
		if err = Build(config); err != nil {
			return err
		}
	}

	deployer := knative.NewDeployer()
	deployer.Verbose = config.Verbose
	deployer.Namespace = viper.GetString("namespace")

	client, err := faas.New(
		faas.WithVerbose(config.Verbose),
		faas.WithDeployer(deployer),
		faas.WithPusher(docker.NewPusher()),
	)
	// TODO: Handle -e flag
	_, err = client.Deploy(f.Name, f.Tag)
	return err
}
