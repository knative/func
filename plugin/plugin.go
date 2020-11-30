package plugin

import (
	"fmt"
	"github.com/boson-project/faas/cmd"
	"knative.dev/client/pkg/kn/plugin"
	"os"
	"runtime/debug"
	"strings"
)

func init() {
	plugin.InternalPlugins = append(plugin.InternalPlugins, &faasPlugin{})
}

type faasPlugin struct {}

func (f *faasPlugin) Name() string {
	return "kn-func"
}

func (f *faasPlugin) Execute(args []string) error {
	rootCmd := cmd.NewRootCmd()
	info, _ := debug.ReadBuildInfo()
	for _, dep := range info.Deps {
		if strings.Contains(dep.Path, "boson-project/faas") {
			cmd.SetMeta("", dep.Version, dep.Sum)
		}
	}
	oldArgs := os.Args
	defer (func() {
		os.Args = oldArgs
	})()
	os.Args = append([]string { "kn-func" }, args...)
	return rootCmd.Execute()
}

// Description for function subcommand visible in 'kn --help'
func (f *faasPlugin) Description() (string, error) {
	return "Function plugin", nil
}

func (f *faasPlugin) CommandParts() []string {
	return []string{ "func"}
}

// Path is empty because its an internal plugins
func (f *faasPlugin) Path() string {
	return ""
}
