package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path"
	"strings"

	"github.com/spf13/cobra"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/knative"
)

func CompleteFunctionList(cmd *cobra.Command, args []string, toComplete string) (strings []string, directive cobra.ShellCompDirective) {
	lister := knative.NewLister("", false)

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

func CompleteRuntimeList(cmd *cobra.Command, args []string, toComplete string, client *fn.Client) (matches []string, directive cobra.ShellCompDirective) {
	runtimes, err := client.Runtimes()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error listing runtimes for flag completion: %v", err)
		return
	}
	for _, runtime := range runtimes {
		if strings.HasPrefix(runtime, toComplete) {
			matches = append(matches, runtime)
		}
	}
	return
}

func CompleteTemplateList(cmd *cobra.Command, args []string, toComplete string, client *fn.Client) (matches []string, directive cobra.ShellCompDirective) {
	repositories, err := client.Repositories().All()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error listing repositories for use in template flag completion: %v", err)
		return
	}
	for _, repository := range repositories {
		templates, err := client.Templates().List(repository.Name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error listing template for use in template flag completion: %v", err)
			return
		}
		matches = append(matches, templates...)
	}
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

func CompleteDeployBuildType(cmd *cobra.Command, args []string, complete string) (buildTypes []string, directive cobra.ShellCompDirective) {
	buildTypes = fn.AllBuildTypes()
	directive = cobra.ShellCompDirectiveDefault
	return
}
