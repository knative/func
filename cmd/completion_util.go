package cmd

import (
	"encoding/json"
	"os"
	"os/user"
	"path"

	"github.com/spf13/cobra"

	fn "github.com/boson-project/func"
	"github.com/boson-project/func/buildpacks"
	"github.com/boson-project/func/knative"
)

func CompleteFunctionList(cmd *cobra.Command, args []string, toComplete string) (strings []string, directive cobra.ShellCompDirective) {
	lister, err := knative.NewLister("")
	if err != nil {
		directive = cobra.ShellCompDirectiveError
		return
	}
	list, err := lister.List(cmd.Context())
	if err != nil {
		directive = cobra.ShellCompDirectiveError
		return
	}

	for _, item := range list {
		strings = append(strings, item.Name)
	}
	directive = cobra.ShellCompDirectiveDefault
	return
}
func CompleteRuntimeList(cmd *cobra.Command, args []string, toComplete string) (strings []string, directive cobra.ShellCompDirective) {
	strings = []string{}
	for lang := range buildpacks.RuntimeToBuildpack {
		strings = append(strings, lang)
	}
	directive = cobra.ShellCompDirectiveDefault
	return
}
func CompleteOutputFormatList(cmd *cobra.Command, args []string, toComplete string) (strings []string, directive cobra.ShellCompDirective) {
	directive = cobra.ShellCompDirectiveDefault
	strings = []string{"plain", "yaml", "xml", "json"}
	return
}

func CompleteRegistryList(cmd *cobra.Command, args []string, toComplete string) (strings []string, directive cobra.ShellCompDirective) {
	directive = cobra.ShellCompDirectiveError
	u, err := user.Current()
	if err != nil {
		return
	}
	file, err := os.Open(path.Join(u.HomeDir, ".docker", "config.json"))
	if err != nil {
		return
	}
	decoder := json.NewDecoder(file)
	var data map[string]interface{}
	err = decoder.Decode(&data)
	if err != nil {
		return
	}
	auth, ok := data["auths"].(map[string]interface{})
	if !ok {
		return
	}
	strings = make([]string, len(auth))
	for reg := range auth {
		strings = append(strings, reg)
	}
	directive = cobra.ShellCompDirectiveDefault
	return
}

func CompleteBuilderList(cmd *cobra.Command, args []string, complete string) (strings []string, directive cobra.ShellCompDirective) {
	directive = cobra.ShellCompDirectiveError

	var (
		err  error
		path string
		f    fn.Function
	)

	path, err = cmd.Flags().GetString("path")
	if err != nil {
		return
	}

	f, err = fn.NewFunction(path)
	if err != nil {
		return
	}

	strings = make([]string, 0, len(f.BuilderMap))
	for name := range f.BuilderMap {
		strings = append(strings, name)
	}

	directive = cobra.ShellCompDirectiveDefault
	return
}
