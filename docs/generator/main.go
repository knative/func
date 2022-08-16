package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io/ioutil"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/cmd"
)

var (
	// Statically-populated build metadata set by `make build`.
	date, vers, hash string
	version          = cmd.Version{
		Date: date,
		Vers: vers,
		Hash: hash,
	}

	// Helper function for indenting template values correctly
	fm = template.FuncMap{
		"indent": func(i int, c string, v string) string {
			indentation := strings.Repeat(c, i)
			return indentation + strings.Replace(v, "\n", "\n"+indentation, -1)
		},
	}

	rootName  = "func"
	targetDir = "docs/reference"
)

// String substitutions in command help docs
type TemplateOptions struct {
	Name    string
	Options string
	Version string
}

// This helper application generates markdown help documents
func main() {
	// Create a new client so that we can get builtin repo options
	factory := cmd.NewClientFactory(func() *fn.Client { return fn.New() })
	client, done := factory(cmd.ClientConfig{}, func(*fn.Client) {})
	defer done()

	// Generate options for templates
	opts, err := cmd.RuntimeTemplateOptions(client)
	if err != nil {
		fmt.Printf("%s\n", err)
	}

	// Initialize an options struct
	templateOptions := TemplateOptions{
		Name:    rootName,
		Options: opts,
		Version: version.StringVerbose(),
	}

	// Create the root command
	var root = cmd.NewRootCmd(cmd.RootCommandConfig{Name: rootName})

	// Write the markdown for the root command
	if err := writeMarkdown(root, rootName, templateOptions); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing help markdown %s", err)
	}

	// Recurse all subcommands and write the markdown for them
	if err := processSubCommands(root, rootName, templateOptions); err != nil {
		fmt.Fprintf(os.Stderr, "Error processing subcommands %s", err)
	}
}

// processSubCommands is a recursive function which writes the markdown text
// for all subcommands of the provided cobra command, prepending the parent
// string to the fiile name, and recursively calls itself for each subcommand.
func processSubCommands(c *cobra.Command, parent string, opts TemplateOptions) error {
	for _, cc := range c.Commands() {
		name := cc.Name()
		if name == "help" {
			continue
		}
		if parent != "" {
			name = parent + "_" + name
		}
		if err := writeMarkdown(cc, name, opts); err != nil {
			return err
		}
		if err := processSubCommands(cc, name, opts); err != nil {
			return err
		}
	}
	return nil
}

// writeMarkdown generates the untemplated markdown string for the given
// command, then does standard template substitution on the generated markdown,
// ultimately writing the markdown file to docs/reference/[command_name].md
func writeMarkdown(c *cobra.Command, name string, opts TemplateOptions) error {
	out := new(bytes.Buffer)
	c.DisableAutoGenTag = true
	err := doc.GenMarkdown(c, out)
	if err != nil {
		return err
	}
	t := template.New("help")
	t.Funcs(fm)
	tpl := template.Must(t.Parse(out.String()))
	out.Reset()
	if err := tpl.Execute(out, opts); err != nil {
		fmt.Fprintf(os.Stderr, "unable to process help text: %v", err)
	}

	ioutil.WriteFile(targetDir+"/"+name+".md", out.Bytes(), 0666)
	return nil
}
