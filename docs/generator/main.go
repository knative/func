package main

import (
	"fmt"
	"io/ioutil"

	"github.com/spf13/cobra"
	"knative.dev/kn-plugin-func/cmd"
)

// This helper application generates markdown help documents
func main() {
	generateMarkdownDocs()
}

type Command struct {
	Impl *cobra.Command
	Name string
}

func writeDoc(c *cobra.Command, name string) {
	fmt.Printf("Generating %s documentation\n", name)
	help := cmd.HelpTemplateFor(c, []string{})
	ioutil.WriteFile("docs/reference/commands/"+name+".md", []byte(help), 0644)
}

// generateMarkdownDocs generates markdown docs for all commands
func generateMarkdownDocs() {
	var c = cmd.NewRootCmd(cmd.RootCommandConfig{Name: "func"})

	for _, x := range c.Commands() {
		// if x.Name() != "create" {
		writeDoc(x, x.Name())
		// subcommands
		for _, y := range x.Commands() {
			if y.Name() != "help" {
				writeDoc(y, x.Name()+"-"+y.Name())
			}
		}
		// }
	}
}
