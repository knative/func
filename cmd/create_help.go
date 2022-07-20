package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"
	"text/template"

	"github.com/charmbracelet/glamour"
	"github.com/spf13/cobra"
	fn "knative.dev/kn-plugin-func"
)

// createLongHelpString is the template string for the create command help text.
// Markdown is used to render headings and source code.
// Text templating is used to dynamically include templates and runtimes
// existing on the user's system
var createLongHelpString = `
## {{.Name}} create

Create a new function project.

### Synopsis

` + "$ `{{.Name}} create [-l|--language] [-t|--template] [-r|--repository] [-c|--confirm]  [-v|--verbose]  [path]`" + `

### Description

	Creates a new function project.

` + "`$ {{.Name}} create -l node -t http`" + `

	Creates a function in the current directory '.' which is written in the
	language/runtime 'node' and handles HTTP events.

	If [path] is provided, the function is initialized at that path, creating
	the path if necessary.

	To complete this command interactivly, use --confirm (-c):
` + "`$ {{.Name}} create -c`" + `

### Templates

	Available language runtimes and templates:

{{ .Options | indent 2 " " | indent 1 "\t" }}

	To install more language runtimes and their templates see ` + "`{{.Name}} repository`." + `

### Examples

- Create a Node.js function (the default language runtime) in the current directory (the default path) which handles http events (the default template).

` + "`$ {{.Name}} create`" + `

- Create a Node.js function in the directory 'myfunc'.

` + "`$ {{.Name}} create myfunc`" + `

- Create a Go function which handles CloudEvents in ./myfunc.

` + "`$ {{.Name}} create -l go -t cloudevents myfunc`" + `
`

// CreateHelp returns the help text for the `create` command
// with template substitutions
func CreateHelp(cmd *cobra.Command, client *fn.Client) string {

	failSoft := failSoftFor(cmd)

	tpl := createHelpTemplate(cmd)

	options, err := runtimeTemplateOptions(client) // human-friendly
	failSoft(err)

	var data = struct {
		Options string
		Name    string
	}{
		Options: options,
		Name:    cmd.Root().Use,
	}

	// execute the template
	var b bytes.Buffer
	failSoft(tpl.Execute(&b, data))
	return b.String()
}

// runCreateHelp is used by the create command to render the help text
func runCreateHelp(cmd *cobra.Command, client *fn.Client) {
	help := CreateHelp(cmd, client)

	// configure markdown renderer
	r, _ := glamour.NewTermRenderer(
		// detect background color and pick either the default dark or light theme
		glamour.WithAutoStyle(),
	)

	// render the markdown template for the CLI
	rendered, err := r.Render(help)
	failSoftFor(cmd)(err)

	// prints the help text to the command stdout
	fmt.Fprint(cmd.OutOrStdout(), rendered)
}

// A function that conditionally writes an error
type ConditionalErrorWriter func(error)

// failSoftFor returns a ConditionalErrorWriter which writes to the provided command's stderr
// Help can not fail when creating the client config (such as on invalid
// flag values) because help text is needed in that situation.   Therefore
// this implementation must be resilient to cfg zero value.
func failSoftFor(cmd *cobra.Command) ConditionalErrorWriter {
	return func(err error) {
		if err != nil {
			fmt.Fprintf(cmd.OutOrStderr(), "error: help text may be partial: %v", err)
		}
	}
}

// Template Helpers
// ---------------

// createHelpTemplate is the template for the create command help
func createHelpTemplate(cmd *cobra.Command) *template.Template {
	t := template.New("help")

	fm := template.FuncMap{
		"indent": func(i int, c string, v string) string {
			indentation := strings.Repeat(c, i)
			return indentation + strings.Replace(v, "\n", "\n"+indentation, -1)
		},
	}
	t.Funcs(fm)
	return template.Must(t.Parse(cmd.Long))
}

// runtimeTemplateOptions is a human-friendly table of valid Language Runtime
// to Template combinations.
func runtimeTemplateOptions(client *fn.Client) (string, error) {
	runtimes, err := client.Runtimes()
	if err != nil {
		return "", err
	}
	builder := strings.Builder{}
	writer := tabwriter.NewWriter(&builder, 0, 0, 3, ' ', 0)

	fmt.Fprint(writer, "Language\tTemplate\n")
	fmt.Fprint(writer, "--------\t--------\n")
	for _, r := range runtimes {
		templates, err := client.Templates().List(r)
		// Not all language packs will have templates for
		// all available runtimes. Without this check
		if err != nil && !errors.Is(err, fn.ErrTemplateNotFound) {
			return "", err
		}
		for _, t := range templates {
			fmt.Fprintf(writer, "%v\t%v\n", r, t) // write tabbed
		}
	}
	writer.Flush()
	return builder.String(), nil
}
