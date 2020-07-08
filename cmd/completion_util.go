package cmd

import (
	"encoding/json"
	"os"
	"os/user"
	"path"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/appsody"
	"github.com/boson-project/faas/knative"
	"github.com/spf13/cobra"
)

func CompleteFunctionList(cmd *cobra.Command, args []string, toComplete string) (strings []string, directive cobra.ShellCompDirective) {
	lister, err := knative.NewLister(faas.DefaultNamespace)
	if err != nil {
		directive = cobra.ShellCompDirectiveError
		return
	}
	s, err := lister.List()
	if err != nil {
		directive = cobra.ShellCompDirectiveError
		return
	}
	strings = s
	directive = cobra.ShellCompDirectiveDefault
	return
}
func CompleteRuntimeList(cmd *cobra.Command, args []string, toComplete string) (strings []string, directive cobra.ShellCompDirective) {
	strings = make([]string, 0, len(appsody.StackShortNames))
	for lang, _ := range appsody.StackShortNames {
		strings = append(strings, lang)
	}
	directive = cobra.ShellCompDirectiveDefault
	return
}
func CompleteOutputFormatList(cmd *cobra.Command, args []string, toComplete string) (strings []string, directive cobra.ShellCompDirective) {
	directive = cobra.ShellCompDirectiveDefault
	strings = []string{"yaml", "xml", "json"}
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
	for reg, _ := range auth {
		strings = append(strings, reg)
	}
	directive = cobra.ShellCompDirectiveDefault
	return
}
