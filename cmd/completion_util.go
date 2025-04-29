package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path"
	"strings"

	"github.com/spf13/cobra"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/knative"
)

func CompleteFunctionList(cmd *cobra.Command, args []string, toComplete string) (strings []string, directive cobra.ShellCompDirective) {
	lister := knative.NewLister(false)

	list, err := lister.List(cmd.Context(), "")
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
	directive = cobra.ShellCompDirectiveError

	lang, err := cmd.Flags().GetString("language")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot list templates: %v", err)
		return
	}
	if lang == "" {
		fmt.Fprintln(os.Stderr, "cannot list templates: language not specified")
		return
	}

	templates, err := client.Templates().List(lang)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot list templates: %v", err)
		return
	}

	directive = cobra.ShellCompDirectiveDefault
	for _, t := range templates {
		if strings.HasPrefix(t, toComplete) {
			matches = append(matches, t)
		}
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

func CompleteBuilderImageList(cmd *cobra.Command, args []string, complete string) (builderImages []string, directive cobra.ShellCompDirective) {
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

	builderImages = make([]string, 0, len(f.Build.BuilderImages))
	for name := range f.Build.BuilderImages {
		if len(complete) == 0 {
			builderImages = append(builderImages, name)
			continue
		}
		if strings.HasPrefix(name, complete) {
			builderImages = append(builderImages, name)
		}
	}

	directive = cobra.ShellCompDirectiveNoFileComp
	return
}

func CompleteBuilderList(cmd *cobra.Command, args []string, complete string) (matches []string, d cobra.ShellCompDirective) {
	d = cobra.ShellCompDirectiveNoFileComp
	matches = []string{}

	if len(complete) == 0 {
		matches = KnownBuilders()
		return
	}

	for _, b := range KnownBuilders() {
		if strings.HasPrefix(b, complete) {
			matches = append(matches, b)
		}
	}

	return
}
