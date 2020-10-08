package plugin

import (
	"github.com/boson-project/faas/cmd"
	"knative.dev/client/pkg/kn/plugin"
	"os"
)

func init() {
	plugin.InternalPlugins = append(plugin.InternalPlugins, &faasPlugin{})
}

type faasPlugin struct {}

func (f *faasPlugin) Name() string {
	return "kn-faas"
}

func (f *faasPlugin) Execute(args []string) error {
    rootCmd := cmd.NewRootCmd()
	oldArgs := os.Args
	defer (func() {
		os.Args = oldArgs
	})()
	os.Args = append([]string { "kn-faas" }, args...)
	return rootCmd.Execute()
}

// Description for faas subcommand visible in 'kn --help'
func (f *faasPlugin) Description() (string, error) {
	return "Function as a Service plugin", nil
}

func (f *faasPlugin) CommandParts() []string {
	return []string{ "faas"}
}

// Path is empty because its an internal plugins
func (f *faasPlugin) Path() string {
	return ""
}
