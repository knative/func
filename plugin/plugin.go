package plugin

import (
	"os"
	"runtime/debug"
	"strings"

	"knative.dev/client/pkg/kn/plugin"

	"github.com/boson-project/func/cmd"
)

func init() {
	plugin.InternalPlugins = append(plugin.InternalPlugins, &funcPlugin{})
}

type funcPlugin struct{}

func (f *funcPlugin) Name() string {
	return "kn-func"
}

func (f *funcPlugin) Execute(args []string) error {
	rootCmd := cmd.NewRootCmd()
	info, _ := debug.ReadBuildInfo()
	for _, dep := range info.Deps {
		if strings.Contains(dep.Path, "boson-project/func") {
			cmd.SetMeta("", dep.Version, dep.Sum)
		}
	}
	oldArgs := os.Args
	defer (func() {
		os.Args = oldArgs
	})()
	os.Args = append([]string{"kn-func"}, args...)
	return rootCmd.Execute()
}

// Description for function subcommand visible in 'kn --help'
func (f *funcPlugin) Description() (string, error) {
	return "Function plugin", nil
}

func (f *funcPlugin) CommandParts() []string {
	return []string{"func"}
}

// Path is empty because its an internal plugins
func (f *funcPlugin) Path() string {
	return ""
}
