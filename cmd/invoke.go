package cmd

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"text/template"

	"github.com/AlecAivazis/survey/v2"
	"github.com/google/uuid"
	"github.com/ory/viper"
	"github.com/spf13/cobra"
	"knative.dev/kn-plugin-func/utils"

	fn "knative.dev/kn-plugin-func"
	fnhttp "knative.dev/kn-plugin-func/http"
	knative "knative.dev/kn-plugin-func/knative"
)

func init() {
	root.AddCommand(NewInvokeCmd(newInvokeClient))
}

type invokeClientFn func(invokeConfig) (*fn.Client, error)

func newInvokeClient(cfg invokeConfig) (*fn.Client, error) {
	describer, err := knative.NewDescriber(cfg.Namespace)
	if err != nil {
		return nil, err
	}
	describer.Verbose = cfg.Verbose

	return fn.New(
		fn.WithDescriber(describer),
		fn.WithTransport(fnhttp.NewRoundTripper()),
		fn.WithVerbose(cfg.Verbose),
	), nil
}

func NewInvokeCmd(clientFn invokeClientFn) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "invoke",
		Short: "Invoke a Function",
		Long: `
NAME
	{{.Prefix}}func invoke - Invoke a Function.

	SYNOPSIS
	{{.Prefix}}func invoke [-t|--target] [-f|--format]
	             [--id] [--source] [--type] [--data] [--file] [--content-type]
	             [-s|--save] [-p|--path] [-c|--confirm] [-v|--verbose]

DESCRIPTION
	Invokes the Function by sending a test request to the currently running
	Function instance, either locally or remote.  If the Function is running
	both locally and remote, the local instance will be invoked.  This behavior
	can be manually overridden using the --target flag.

	Functions are invoked with a test data structure consisting of five values:
		id:            A unique identifer for the request.
		source:        A sender name for the request (sender).
		type:          A type for the request.
		data:          Data (content) for this request.
		content-type:  The MIME type of the value contained in 'data'.

	The values of these parameters can be individually altered from their defaults
	using their associated flags. Data can also be provided from a file using the
	--file flag.

	Invocation Target
	  The Function instance to invoke can be specified using the --target flag
	  which accepts the values "local", "remote", or <URL>.  By default the
	  local Function instance is chosen if running (see {{.Prefix}}func run).
	  To explicitly target the remote (deployed) Function:
	    {{.Prefix}}func invoke --target=remote
	  To target an arbitrary endpont, provide a URL:
	    {{.Prefix}}func invoke --target=https://myfunction.example.com

	Invocation Data
	  Providing a filename in the --file flag will base64 encode its contents
	  as the "data" parameter sent to the Function.  The value of --content-type
	  should be set to the type from the source file.  For example, the following
	  would send a JPEG base64 encoded in the "data" POST parameter:
	    {{.Prefix}}func invoke --file=example.jpeg --content-type=image/jpeg

	Message Format
	  By default Functions are sent messages which match the invocation format
	  of the template they were created using; for example "http" or "cloudevent".
	  To override this behavior, use the --format (-f) flag.
	    {{.Prefix}}func invoke -f=cloudevent -t=http://my-sink.my-cluster

EXAMPLES

	o Invoke the default (local or remote) running Function with default values
	  $ {{.Prefix}}func invoke

	o Run the Function locally and then invoke it with a test request:
	  (run in two terminals or by running the first in the background)
	  $ {{.Prefix}}func run
	  $ {{.Prefix}}func invoke

	o Deploy and then invoke the remote Function:
	  $ {{.Prefix}}func deploy
	  $ {{.Prefix}}func invoke

	o Invoke a remote (deployed) Function when it is already running locally:
	  (overrides the default behavior of preferring locally running instances)
	  $ {{.Prefix}}func invoke --target=remote

	o Specify the data to send to the Function as a flag
	  $ {{.Prefix}}func invoke --data="Hello World!"

	o Send a JPEG to the Function
	  $ {{.Prefix}}func invoke --file=example.jpeg --content-type=image/jpeg

	o Invoke an arbitrary endpoint (HTTP POST)
		$ {{.Prefix}}func invoke --target="https://my-event-broker.example.com"

	o Invoke an arbitrary endpoint (CloudEvent)
		$ {{.Prefix}}func invoke -f=cloudevent -t="https://my-event-broker.example.com"

`,
		SuggestFor: []string{"emit", "emti", "send", "emit", "exec", "nivoke", "onvoke", "unvoke", "knvoke", "imvoke", "ihvoke", "ibvoke"},
		PreRunE:    bindEnv("path", "format", "target", "id", "source", "type", "data", "content-type", "file", "confirm", "namespace"),
	}

	// Flags
	cmd.Flags().StringP("path", "p", cwd(), "Path to the Function which should have its instance invoked. (Env: $FUNC_PATH)")
	cmd.Flags().StringP("format", "f", "", "Format of message to send, 'http' or 'cloudevent'.  Default is to choose automatically. (Env: $FUNC_FORMAT)")
	cmd.Flags().StringP("target", "t", "", "Function instance to invoke.  Can be 'local', 'remote' or a URL.  Defaults to auto-discovery if not provided. (Env: $FUNC_TARGET)")
	cmd.Flags().StringP("id", "", uuid.NewString(), "ID for the request data. (Env: $FUNC_ID)")
	cmd.Flags().StringP("source", "", fn.DefaultInvokeSource, "Source value for the request data. (Env: $FUNC_SOURCE)")
	cmd.Flags().StringP("type", "", fn.DefaultInvokeType, "Type value for the request data. (Env: $FUNC_TYPE)")
	cmd.Flags().StringP("content-type", "", fn.DefaultInvokeContentType, "Content Type of the data. (Env: $FUNC_CONTENT_TYPE)")
	cmd.Flags().StringP("data", "", fn.DefaultInvokeData, "Data to send in the request. (Env: $FUNC_DATA)")
	cmd.Flags().StringP("file", "", "", "Path to a file to use as data. Overrides --data flag and should be sent with a correct --content-type. (Env: $FUNC_FILE)")
	cmd.Flags().BoolP("confirm", "c", false, "Prompt to confirm all options interactively. (Env: $FUNC_CONFIRM)")
	setNamespaceFlag(cmd)

	// Help Action
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		runInvokeHelp(cmd, args, clientFn)
	})

	// Run Action
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runInvoke(cmd, args, clientFn)
	}

	return cmd
}

// Run
func runInvoke(cmd *cobra.Command, args []string, clientFn invokeClientFn) (err error) {
	// Gather flag values for the invocation
	cfg, err := newInvokeConfig(clientFn)
	if err != nil {
		return
	}

	// Client instance from env vars, flags, args and user prompts (if --confirm)
	client, err := clientFn(cfg)
	if err != nil {
		return err
	}

	// Message to send the running Function built from parameters gathered
	// from the user (or defaults)
	m := fn.InvokeMessage{
		ID:          cfg.ID,
		Source:      cfg.Source,
		Type:        cfg.Type,
		ContentType: cfg.ContentType,
		Data:        cfg.Data,
		Format:      cfg.Format,
	}

	// If --file was specified, use its content for message data
	if cfg.File != "" {
		content, err := os.ReadFile(cfg.File)
		if err != nil {
			return err
		}
		m.Data = base64.StdEncoding.EncodeToString(content)
	}

	// Invoke
	err = client.Invoke(cmd.Context(), cfg.Path, cfg.Target, m)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStderr(), "Invoked %v\n", cfg.Target)
	return
}

func runInvokeHelp(cmd *cobra.Command, args []string, clientFn invokeClientFn) {
	var (
		body = cmd.Long + "\n\n" + cmd.UsageString()
		t    = template.New("invoke")
		tpl  = template.Must(t.Parse(body))
	)

	var data = struct {
		Prefix string
	}{
		Prefix: pluginPrefix(),
	}

	if err := tpl.Execute(cmd.OutOrStdout(), data); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "unable to display help text: %v", err)
	}
}

type invokeConfig struct {
	Path        string
	Target      string
	Format      string
	ID          string
	Source      string
	Type        string
	Data        string
	ContentType string
	File        string
	Namespace   string
	Confirm     bool
	Verbose     bool
}

func newInvokeConfig(clientFn invokeClientFn) (cfg invokeConfig, err error) {
	cfg = invokeConfig{
		Path:        viper.GetString("path"),
		Target:      viper.GetString("target"),
		ID:          viper.GetString("id"),
		Source:      viper.GetString("source"),
		Type:        viper.GetString("type"),
		Data:        viper.GetString("data"),
		ContentType: viper.GetString("content-type"),
		File:        viper.GetString("file"),
		Confirm:     viper.GetBool("confirm"),
		Verbose:     viper.GetBool("verbose"),
	}

	// If file was passed, read it in as data
	if cfg.File != "" {
		b, err := ioutil.ReadFile(cfg.File)
		if err != nil {
			return cfg, err
		}
		cfg.Data = string(b)
	}

	// if not in confirm/prompting mode, the cfg structure is complete.
	if !cfg.Confirm {
		return
	}

	// Client instance for use during prompting.
	client, err := clientFn(cfg)
	if err != nil {
		return
	}

	// If in interactive terminal mode, prompt to modify defaults.
	if interactiveTerminal() {
		return cfg.prompt(client)
	}

	// Confirming, but noninteractive, is essentially a selective verbose mode
	// which prints out the effective values of config as a confirmation.
	fmt.Printf("Path: %v\n", cfg.Path)
	fmt.Printf("Target: %v\n", cfg.Target)
	fmt.Printf("ID: %v\n", cfg.ID)
	fmt.Printf("Source: %v\n", cfg.Source)
	fmt.Printf("Type: %v\n", cfg.Type)
	fmt.Printf("Data: %v\n", cfg.Data)
	fmt.Printf("Content Type: %v\n", cfg.ContentType)
	fmt.Printf("File: %v\n", cfg.File)
	return
}

func (c invokeConfig) prompt(client *fn.Client) (invokeConfig, error) {
	var qs []*survey.Question

	// First get path to effective Function
	qs = []*survey.Question{
		{
			Name: "Path",
			Prompt: &survey.Input{
				Message: "Function Path:",
				Default: c.Path,
			},
			Validate: func(val interface{}) error {
				if val.(string) != "" {
					derivedName, _ := deriveNameAndAbsolutePathFromPath(val.(string))
					return utils.ValidateFunctionName(derivedName)
				}
				return nil
			},
			Transform: func(ans interface{}) interface{} {
				if ans.(string) != "" {
					_, absolutePath := deriveNameAndAbsolutePathFromPath(ans.(string))
					return absolutePath
				}
				return ""
			},
		},
	}
	if err := survey.Ask(qs, &c); err != nil {
		return c, err
	}

	qs = []*survey.Question{
		{
			Name: "Target",
			Prompt: &survey.Input{
				Message: "(Optional) Target ('local', 'remote' or URL).  If not provided, local will be preferred over remote.",
				Default: "",
			},
		},
		{
			Name: "Format",
			Prompt: &survey.Select{
				Message: "(Optional) Format Override",
				Options: []string{"", "http", "cloudevent"},
				Default: c.Format,
			},
		},
	}
	if err := survey.Ask(qs, &c); err != nil {
		return c, err
	}

	// Prompt for the next set of values, with defaults set first by the Function
	// as it exists on disk, followed by environment variables, and finally flags.
	// user interactive prompts therefore are the last applied, and thus highest
	// precidence values.
	qs = []*survey.Question{
		{
			Name: "ID",
			Prompt: &survey.Input{
				Message: "Data ID",
				Default: c.ID,
			},
		}, {
			Name: "Source",
			Prompt: &survey.Input{
				Message: "Data Source",
				Default: c.Source,
			},
		}, {
			Name: "Type",
			Prompt: &survey.Input{
				Message: "Data Type",
				Default: c.Type,
			},
		}, {
			Name: "File",
			Prompt: &survey.Input{
				Message: "(Optional) Load Data Content from File",
				Default: c.File,
			},
		},
	}
	if err := survey.Ask(qs, &c); err != nil {
		return c, err
	}

	// If the user did not specify a file for data content, prompt for it
	if c.File == "" {
		qs = []*survey.Question{
			{
				Name: "Data",
				Prompt: &survey.Input{
					Message: "Data Content",
					Default: c.Data,
				},
			},
		}
		if err := survey.Ask(qs, &c); err != nil {
			return c, err
		}
	}

	// Finally, allow mutation of the data content type.
	contentTypeMessage := "Content type of data"
	if c.File != "" {
		contentTypeMessage = "Content type of file"
	}
	qs = []*survey.Question{
		{
			Name: "ContentType",
			Prompt: &survey.Input{
				Message: contentTypeMessage,
				Default: c.ContentType,
			},
		}}
	if err := survey.Ask(qs, &c); err != nil {
		return c, err
	}

	return c, nil
}
