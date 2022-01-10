package cmd

import (
	"fmt"
	"io/ioutil"
	"text/template"

	"github.com/AlecAivazis/survey/v2"
	"github.com/ory/viper"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"knative.dev/kn-plugin-func/utils"

	fn "knative.dev/kn-plugin-func"
	fnhttp "knative.dev/kn-plugin-func/http"
)

func init() {
	root.AddCommand(NewInvokeCmd(newInvokeClient))
}

type invokeClientFn func(invokeConfig) *fn.Client

func newInvokeClient(cfg invokeConfig) *fn.Client {
	return fn.New(
		fn.WithTransport(fnhttp.NewRoundTripper()),
		fn.WithVerbose(cfg.Verbose),
	)
}

func NewInvokeCmd(clientFn invokeClientFn) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "invoke",
		Short: "Invoke a Function",
		Long: `
NAME
	{{.Prefix}}func invoke - Invoke a Function.

	SYNOPSIS
	{{.Prefix}}func invoke [-t|--target]
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
		type:          A type for this request.
		data:          Data (content) for this request.
		content-type:  The MIME type of the value contained in 'data'.

	The values of these parameters can be individually altered from their defaults
	using their associated flags. Data can also be provided from file with --file.

	The Function template chosen at time of Function creation determines how this
	data arrives.  It can be in the form of an HTTP POST or a Cloud Event

	Invocation Target
	  The Function instance to invoke can be specified using the --target flag
	  which accepts the values "local", "remote", or <URL>.  By default the
	  local Function instance is chosen if running (see {{.Prefix}}func run).
	  To explicitly target the remote (deployed) Function:
	    func invoke --target=remote
	  To target an arbitrary endpont, provide a URL:
	    func invoke --target=https://myfunction.example.com

EXAMPLES

	o Invoke the default (local or remote) running Function with default values
	  $ {{.Prefix}}func invoke

	o Run the Function locally and then invoke it with a test request:
	  $ {{.Prefix}}func run
	  $ {{.Prefix}}func invoke

	o Deploy and invoke the remote Function:
	  $ {{.Prefix}}func deploy
	  $ {{.Prefix}}func invoke

	o Invoke a remote (deployed) Function when it is already running locally:
	  (overrides the default behavior of preferring locally running instances)
	  $ {{.Prefix}}func invoke --target=remote

	o Specify the data to send to the Function as a flag
	  $ {{.Prefix}}func invoke --data="Hello World!"

	o Send a JPEG to the Function
	  $ {{.Prefix}}func invoke --file=example.jpeg --content-type=image/jpeg

	o Invoke an arbitrary endpoint
		$ {{.Prefix}}func invoke --target="https://my-event-broker.example.com"

`,
		SuggestFor: []string{"emit", "emti", "send", "emit", "exec", "nivoke", "onvoke", "unvoke", "knvoke", "imvoke", "ihvoke", "ibvoke"},
		PreRunE:    bindEnv("path", "target", "id", "source", "type", "data", "content-type", "file"),
	}

	// Flags
	cmd.Flags().StringP("path", "p", cwd(), "Path to the Function which should have its instance invoked (Env: $FUNC_PATH)")
	cmd.Flags().StringP("target", "t", "", "Function instance to invoke.  Can be 'local', 'remote' or a URL.  Defaults to auto-discovery if not provided (Env: $FUNC_TARGET)")
	cmd.Flags().BoolP("confirm", "c", false, "Prompt to confirm all options interactively (Env: $FUNC_CONFIRM)")
	cmd.Flags().StringP("id", "", "", "ID for the request data. (Env: $FUNC_ID)")
	cmd.Flags().StringP("source", "", "", "Source value for the request data. (Env: $FUNC_SOURCE)")
	cmd.Flags().StringP("type", "", "", "Type value for the request data. (Env: $FUNC_TYPE)")
	cmd.Flags().StringP("data", "", "", "Data to send in the request. (Env: $FUNC_DATA)")
	cmd.Flags().StringP("content-type", "", "", "Content Type of the data. (Env: $FUNC_CONTENT_TYPE)")
	cmd.Flags().StringP("file", "", "", "Path to a file containg data to send. Eclusive with --data flag and requres correct --content-type. (Env: $FUNC_FILE)")

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

	// Client instance from env vars, flags, args and user promtps (if --confirm)
	client := clientFn(cfg)

	// A deeper validation than that which is performed when instantiating the
	// client with the raw config above.
	if err = cfg.Validate(); err != nil {
		return
	}

	// Invoke
	err = client.Invoke(cmd.Context(), cfg.Path, cfg.Target, fn.InvokeMessage{
		ID:          cfg.ID,
		Source:      cfg.Source,
		Type:        cfg.Type,
		ContentType: cfg.ContentType,
		Data:        cfg.Data,
	})
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
	ID          string
	Source      string
	Type        string
	Data        string
	ContentType string
	File        string
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
	// See .Validate for file/data exclusivity checks
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
	client := clientFn(cfg)

	// If in interactive terminal mode, prompt to modify defaults.
	if interactiveTerminal() {
		return cfg.prompt(client)
	}

	// Confirming, but noninteractive
	// Print out the final values as confirmation
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

// Validate the current state of config.
func (c invokeConfig) Validate() error {
	if c.Data != "" && c.File != "" {
		return errors.New("Only one of --data or --file may be specified")
	}
	return nil
}

func (c invokeConfig) prompt(client *fn.Client) (invokeConfig, error) {
	var qs []*survey.Question

	// First get path to effective Function
	qs = []*survey.Question{
		{
			Name: "Path",
			Prompt: &survey.Input{
				Message: "Function Path (optional):",
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
				Message: "(optional) Target Function ('local', 'remote' or URL endpoint. default: auto)",
				Default: c.Target,
			},
		},
	}
	if err := survey.Ask(qs, &c); err != nil {
		return c, err
	}

	// TODO: load Function if path defined

	// Apply Overrides
	// The current state of the config includes environment variables and
	// flag values.  These override the settings defined in the function.
	/*
		if f, err := applyInvocationOverrides(f, c); err != nil {
			return
		}
	*/

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
				Message: "Load Data Content from File (optional)",
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
	qs = []*survey.Question{
		{
			Name: "ContentType",
			Prompt: &survey.Input{
				Message: "Content Type of Data",
				Default: c.ContentType,
			},
		}}
	if err := survey.Ask(qs, &c); err != nil {
		return c, err
	}

	return c, nil
}
