package cmd

import (
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
func CompleteLanguageList(cmd *cobra.Command, args []string, toComplete string) (strings []string, directive cobra.ShellCompDirective) {
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