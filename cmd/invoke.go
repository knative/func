package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/google/uuid"
	"github.com/ory/viper"
	"github.com/spf13/cobra"
	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/invoker"
	"knative.dev/kn-plugin-func/knative"
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

	return fn.New(
		fn.WithDescriber(describer),
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
	{{.Prefix}}func invoke [-t|--target]
	             [--id] [--source] [--type] [--data] [--file] [--content-type]
	             [-r|--request] [-s|--save]
	             [-p|--path] [-c|--confirm] [-v|--verbose]

DESCRIPTION
	Invokes the Function by sending a test request to the currently running
	Function instance, either locally or remote.  If the Function is running
	both locally and remote, the local instance will be invoked.  This behavior
	can be manually overridden using the --target flag.
$ func run $ func invoke

	Functions are invoked with a test data structure consisting of five values: 
		id:            A unique identifer for the request.
		source:        Arbitrary sender name for the request (sender).
		type:          Arbitrary type for this request.
		data:          Arbitrary data (content) for this request.
		content-type:  The MIME type of the value contained in 'data'.

	The values of these parameters can be individually altered from their defaults
	using their associated flags. Data can also be provided from file with --file.

	The Function template chosen at time of Function creation determines how this
	data arrives.  It can be in the form of HTTP GET or POST parameters, a Cloud
	Event, or an entirely custom request.  The current Function is configured to
	be invoked as follows:
	  Function Name:
	  Function Path:
	  Request Method:

	Invocation Target
	  The Function to invoke can be specified using the --target flag which
	  accepts the values "local", "remote", or <URL>.  By default the locally-
	  running Function instance is chosen if one is running (see {{.Prefix}}func run).
	  To explicitly target a remote (deployed) Function:
	    func invoke --target=remote
	  To target an arbitrary endpont, provide a URL:
	    func invoke --target=https://my-function.example.com

	Custom Requests
	  To provide a raw HTTP request to the Function, use the --request flag which
	  will read an HTTP request in from STDIN.  The request is parsed as a
	  template and can access the function via a '.f' member.

	Saving Requests for Repeated Invocations
	  The final request sent to the Function can be manipulated using individual
	  flags or even by providing a raw HTTP request as a template (see above).
	  The final values of this request can be saved for future invocations using
	  the --save flag.

EXAMPLES

	o Run the Function locally and then invoke it with a test request:
	  $ {{.Prefix}}func run
	  $ {{.Prefix}}func invoke

	o Deploy and invoke the remote Function:
	  $ {{.Prefix}}func deploy
	  $ {{.Prefix}}func invoke

	o Invoke a remote (deployed) Function when it is already running locally:
	  (overrides the default behavior of preferring locally running instances)
	  $ {{.Prefix}}func invoke --target=remote

	o Specify the data to send to the Function, saving for future invocations:
	  $ {{.Prefix}}func invoke --data="Hello World!" --save

	o Send a JPEG to the Function
	  $ {{.Prefix}}func invoke --file=example.jpeg --content-type=image/jpeg
		`,
		SuggestFor: []string{"emit", "emti", "send", "emit", "exec", "nivoke", "onvoke", "unvoke", "knvoke", "imvoke", "ihvoke", "ibvoke"},
		PreRunE:    bindEnv("target", "id", "source", "type", "data", "file", "content-type", "save", "path", "confirm", "verbose"),
	}

	// Flags
	cmd.Flags().StringP("path", "p", cwd(), "Path to the Function which should have its instance invoked (Env: $FUNC_PATH)")
	cmd.Flags().StringP("target", "t", "", "Function instance to invoke.  Can be 'local', 'remote' or a URL. (Env: $FUNC_TARGET)")
	cmd.Flags().StringP("id", "", uuid.NewString(), "ID for the request data. (Env: $FUNC_ID)")
	cmd.Flags().StringP("source", "", invoker.DefaultSource, "Source value for the request data. (Env: $FUNC_SOURCE)")
	cmd.Flags().StringP("type", "", invoker.DefaultType, "Type value for the request data. (Env: $FUNC_TYPE)")
	cmd.Flags().StringP("data", "", invoker.DefaultData, "Data sent in the request. (Env: $FUNC_DATA)")
	cmd.Flags().StringP("content-type", "", invoker.DefaultContentType, "Content Type of the data. (Env: $FUNC_CONTENT_TYPE)")
	cmd.Flags().StringP("file", "", "", "Path to a file containg data to send. Eclusive with --data flag and requres correct --content-type. (Env: $FUNC_FILE)")
	cmd.Flags().BoolP("save", "", false, "Save request values specified as the new defaults for future invocations.  (Env: $FUNC_SAVE)")
	cmd.Flags().BoolP("request", "", false, "Provide a raw HTTP request manually via STDIN in place of other options. (Env: $FUNC_REQUEST)")

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

	cfg, err := newInvokeConfig(clientFn)
	if err != nil {
		return
	}

	client := clientFn(cfg)

	// A deeper validation than that which is performed when instantiating the
	// client with the raw config above.
	if err = cfg.Validate(); err != nil {
		return
	}

	// Load Function
	// This path has gone through defaulting, env/flag overrieds and prompts.
	if f, err = fn.NewFunction(cfg.Path); err != nil {
		return
	}

	// Apply Overrides
	// Config now includes final values for the invocation fields, including
	// env/flags and prompts.  Apply those settings to the function just loaded
	// from the final path.
	if f, err := applyInvocationOverrides(f, cfg); err != nil {
		return
	}

	// Save
	// Persist the final value of F if saving these settings
	if cfg.Save {
		f.Save()
	}

	// Invoke
	err = client.Invoke(cmd.Context(), f,
		fn.InvokeWithRequest(cfg.Request))
	if err != nil {
		return err
	}

	// Confirm
	fmt.Fprintf(cmd.OutOrStderr(), "Invoked %v\n", f.Name)

	/*
		// Determine the final endpoint, taking into account the special value "local",
		// and sampling the function's current route if not explicitly provided
		endpoint, err := endpoint(cmd.Context(), config)
		if err != nil {
			return err
		}
	*/
}

func runInvokeHelp(cmd *cobra.Command, args []string, clientFn invokeClientFn) {
	// Error-tolerant implementation:
	// Help can not faile when creating the client because help is needed in
	// precisely that situation.  Therefore the impl is resilient to zero values
	// etc.
	failSoft := func(err error) {
		if err != nil {
			fmt.Fprintf(cmd.OutOrStderr(), "error: help text may be partial: %v", err)
		}
	}

	tpl := invokeHelpTemplate(cmd)

	cfg, err := newInvokeConfig(args, clientFn)
	failSoft(err)

	client := clientFn(cfg)

	f, err := fn.NewFunction(cfg.Path)
	failSoft(err)

	var data = struct {
		F      fn.Function
		Prefix string
	}{
		F:      f,
		Prefix: pluginPrefix(),
	}

	if err := tpl.Execute(cmd.OutOrStdout(), data); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "unable to display help text: %v", err)
	}
}

type invokeConfig struct {
	Path        string
	Target      string
	Id          string
	Source      string
	Type        string
	Data        string
	ContentType string
	File        string
	Save        bool
	Verbose     bool
}

func newInvokeConfig() (cfg invokeConfig, err error) {
	cfg = invokeConfig{
		Path:        viper.GetString("path"),
		Target:      viper.GetString("target"),
		Id:          viper.GetString("id"),
		Source:      viper.GetString("source"),
		Type:        viper.GetString("type"),
		Data:        viper.GetString("data"),
		ContentType: viper.GetString("content-type"),
		File:        viper.GetString("file"),
		Save:        viper.GetBool("save"),
		Verbose:     viper.GetBool("verbose"),
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
	fmt.Printf("ID: %v\n", cfg.Id)
	fmt.Printf("Source: %v\n", cfg.Source)
	fmt.Printf("Type: %v\n", cfg.Type)
	fmt.Printf("Content Type: %v\n", cfg.ContentType)
	fmt.Printf("File: %v\n", cfg.File)
	fmt.Printf("Save: %v\n", cfg.Save)
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
				Message: "Function Path:",
				Defaut:  c.Path,
			},
		},
	}
	if err := survey.Ask(qs, &c); err != nil {
		return c, err
	}

	// Once we have the final path to the Function,
	// load it and use its state to determine target and defaults for subsequent
	// prompts.
	f, err := fn.NewFunction(c.Path)
	if err != nil {
		return c, err
	}
	if client.IsRunningLocally(f) {
		c.Target = "local"
	} else if client.IsRunningRemote(f) {
		c.Target = "remote"
	} else {
		c.Target = ""
	}
	qs = []*survey.Question{
		{
			Name: "Target",
			Prompt: &survey.Input{
				Message: "Target Function",
				Default: c.Target,
			},
		},
	}
	if err := survey.Ask(qs, &c); err != nil {
		return c, err
	}

	// if --request (raw), skip all the rest of the questions below
	// because they are all overridden by the value of the HTTP request sent
	// in via STDIN
	if c.Request {
		return
	}

	// Apply Overrides
	// The current state of the config includes environment variables and
	// flag values.  These override the settings defined in the function.
	if f, err := applyInvocationOverrides(f, c); err != nil {
		return
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
				Default: c.Id,
			},
		}, {
			Name: "Source",
			Prompt: &survey.Input{
				"Data Source",
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

// endpoint returns the final effective endpoint.
// By default, the contextually active Function is queried for it's current
// address (route).
// If "local" is specified in cfg.Sink, localhost is used.
// Otherwise the value of Sink is used verbatim if defined.
func endpoint(ctx context.Context, cfg invokeConfig) (url string, err error) {
	var (
		f fn.Function
		d fn.Describer
		i fn.Info
	)

	// If a sink was not provided, default to localhost
	if cfg.Sink == "" {
		return "http://localhost:8080", nil
	}

	// If the special value "cluster" was not provided,
	// this implies an explicit sink URI, use it.
	if cfg.Sink != "cluster" {
		return cfg.Sink, nil
	}

	// The special value "cluster", use the route to the currently
	// contectually active function
	if f, err = fn.NewFunction(cfg.Path); err != nil {
		return
	}

	// TODO: Decide what happens if the function hasn't been deployed but they
	// don't run with --local=true.  Perhaps an error in .Validate()?
	if d, err = knative.NewDescriber(""); err != nil {
		return
	}

	// Get the current state of the function.
	if i, err = d.Describe(ctx, f.Name); err != nil {
		return
	}

	// Probably wise to be defensive here:
	if len(i.Routes) == 0 {
		err = errors.New("function has no active routes")
		return
	}

	// The first route should be the destination.
	return i.Routes[0], nil
}
