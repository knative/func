package main

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"

	"knative.dev/func/cmd"
	fn "knative.dev/func/pkg/functions"
)

var (
	// Helper function for indenting template values correctly
	fm = template.FuncMap{
		"indent": func(i int, c string, v string) string {
			indentation := strings.Repeat(c, i)
			return indentation + strings.Replace(v, "\n", "\n"+indentation, -1)
		},
		"rootCmdUse": func() string {
			return rootName
		},
	}

	rootName  = "func"
	targetDir = "docs/reference"
)

// String substitutions in command help docs
type TemplateOptions struct {
	Name    string
	Options string
	Use     string
}

// This helper application generates markdown help documents
func main() {
	// Ignore global config.yaml if it exists
	defer ignoreConfigEnv()()

	// Create a new client so that we can get builtin repo options
	client := fn.New()

	// Generate options for templates
	opts, err := cmd.RuntimeTemplateOptions(client)
	if err != nil {
		fmt.Printf("%s\n", err)
	}

	// Initialize an options struct
	templateOptions := TemplateOptions{
		Name:    rootName,
		Options: opts,
		Use:     rootName,
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

// ignoreConfigEnv sets FUNC_CONFIG_FILE to a nonexistent path and
// returns a function which undoes this change.
func ignoreConfigEnv() (done func()) {
	var (
		env   = "FUNC_CONFIG_FILE"
		v, ok = os.LookupEnv(env)
	)
	os.Setenv(env, filepath.Join(os.TempDir(), "nonexistent.yaml"))
	return func() {
		if ok {
			os.Setenv(env, v)
		} else {
			os.Unsetenv(env)
		}
	}
}

// processSubCommands is a recursive function which writes the markdown text
// for all subcommands of the provided cobra command, prepending the parent
// string to the file name, and recursively calls itself for each subcommand.
func processSubCommands(c *cobra.Command, parent string, opts TemplateOptions) error {
	for _, cc := range c.Commands() {
		name := cc.Name()
		if name == "help" {
			continue
		}
		if parent != "" {
			name = parent + "_" + name
		}
		opts.Use = cc.Use
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

	re := regexp.MustCompile(`[^\S\r\n]+\n`)
	data := re.ReplaceAll(out.Bytes(), []byte{'\n'}) // trim white spaces before EOL

	if err := os.WriteFile(targetDir+"/"+name+".md", data, 0666); err != nil {
		return err
	}
	return nil
}
