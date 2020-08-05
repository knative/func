package cmd

import (
	"fmt"
	"os"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/buildpacks"
	"github.com/boson-project/faas/docker"
	"github.com/ory/viper"
	"github.com/spf13/cobra"
)

var buildCmd = &cobra.Command{
	Use:        "build",
	Short:      "Build an existing function project as an OCI image",
	SuggestFor: []string{"biuld", "buidl", "built", "image"},
	// TODO: Add completions for build
	// ValidArgsFunction: CompleteRuntimeList,
	RunE: buildImage,
	PreRunE: func(cmd *cobra.Command, args []string) (err error) {
		flags := []string{"path", "tag", "push"}
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

	root.AddCommand(buildCmd)
	buildCmd.Flags().StringP("path", "p", cwd, "Path to the function project directory")
	buildCmd.Flags().StringP("tag", "t", "", "Specify an image tag, for example quay.io/myrepo/project.name:latest")
}

type buildConfig struct {
	Verbose bool
	Path    string
	Tag     string
	Push    bool
}

func buildImage(cmd *cobra.Command, args []string) (err error) {
	var config = buildConfig{
		Verbose: viper.GetBool("verbose"),
		Path:    viper.GetString("path"),
		Tag:     viper.GetString("tag"),
	}
	return Build(config)
}

// FunctionConfigForBuild accepts a buildConfig and
// uses this to load a faas.Function. If the config contains
// a value for the image tag, this will be used and
// subsequently written to the config file.
func FunctionConfigForBuild(config buildConfig) (f *faas.Function, err error) {
	f, err = faas.NewFunction(config.Path)
	if err != nil {
		return nil, err
	}

	if f.Name == "" || f.Runtime == "" {
		return nil, fmt.Errorf("Unable to find a function project at %s", config.Path)
	}

	// Allow CLI to override pre-configured tag name
	var tag = config.Tag
	if tag == "" {
		// get tag from config file
		tag = f.Tag

		if tag == "" {
			tag = fmt.Sprintf("quay.io/%s:latest", f.Name)
			fmt.Printf("No tag provided, using %s\n", tag)
		}
	}

	// Write the image tag to the function configuration
	f.Tag = tag
	if err = f.WriteConfig(); err != nil {
		fmt.Printf("Error writing configuration %v\n", err)
		return nil, err
	}

	return f, nil
}

// Build will build a function project image and optionally
// push it to a remote registry
func Build(config buildConfig) (err error) {
	f, err := FunctionConfigForBuild(config)
	if err != nil {
		return err
	}
	builder := buildpacks.NewBuilder(f.Tag)
	builder.Verbose = config.Verbose

	var client *faas.Client
	if config.Push {
		client, err = faas.New(
			faas.WithVerbose(config.Verbose),
			faas.WithBuilder(builder),
			faas.WithPusher(docker.NewPusher()),
		)
	} else {
		client, err = faas.New(
			faas.WithVerbose(config.Verbose),
			faas.WithBuilder(builder),
		)
	}
	if err != nil {
		return err
	}

	_, err = client.Build(f.Root)
	return err // will be nil if no error
}
