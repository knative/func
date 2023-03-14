package cmd

import (
	"github.com/ory/viper"
	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
)

// TODO: once functional break into a config for each of the metadata commands
// labels, envs, volumes

type metadataConfig struct {
	config.Global // includes verbose
	Path          string
}

func newMetadataConfig() metadataConfig {
	return metadataConfig{
		Global: config.Global{
			Verbose: viper.GetBool("verbose"),
			Output:  viper.GetString("output"),
		},
		Path: viper.GetString("path"),
	}
}

func (c metadataConfig) Validate() error {
	// TODO: validate cascate upwards like the constructor.
	// c.Global.Validat() // should validate .Output
	return nil
}

func (c metadataConfig) Configure(f fn.Function) (fn.Function, error) {
	f = c.Global.Configure(f)
	return f, nil
}

func (c metadataConfig) Prompt() (metadataConfig, error) {
	if !interactiveTerminal() {
		return c, nil
	}
	//
	// TODO: move all the prompts here
	//
	return c, nil
}
