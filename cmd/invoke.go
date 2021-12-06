package cmd

import (
	"errors"
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/ory/viper"
	"github.com/spf13/cobra"
	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/utils"
)

func init() {
	root.AddCommand(NewInvokeCmd(newInvokeClient))
}

type invokeClientFn func(invokeConfig) *fn.Client

func newInvokeClient(cfg invokeConfig) *fn.Client {
	return fn.New(fn.WithVerbose(cfg.Verbose))
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
	    func invoke --target=https://myfunction.example.com

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
		PreRunE:    bindEnv("path", "target", "id", "source", "type", "data", "content-type", "file", "save", "confirm"),
	}

	// Flags
	cmd.Flags().StringP("path", "p", cwd(), "Path to the Function which should have its instance invoked (Env: $FUNC_PATH)")
	cmd.Flags().StringP("target", "t", "", "Function instance to invoke.  Can be 'local', 'remote' or a URL. (Env: $FUNC_TARGET)")
	cmd.Flags().BoolP("confirm", "c", false, "Prompt to confirm all options interactively (Env: $FUNC_CONFIRM)")
	cmd.Flags().StringP("id", "", "", "ID for the request data. (Env: $FUNC_ID)")
	cmd.Flags().StringP("source", "", "", "Source value for the request data. (Env: $FUNC_SOURCE)")
	cmd.Flags().StringP("type", "", "", "Type value for the request data. (Env: $FUNC_TYPE)")
	cmd.Flags().StringP("data", "", "", "Data to send in the request. (Env: $FUNC_DATA)")
	cmd.Flags().StringP("content-type", "", "", "Content Type of the data. (Env: $FUNC_CONTENT_TYPE)")
	cmd.Flags().StringP("file", "", "", "Path to a file containg data to send. Eclusive with --data flag and requres correct --content-type. (Env: $FUNC_FILE)")
	cmd.Flags().BoolP("save", "", false, "Save request values specified as the new defaults for future invocations.  (Env: $FUNC_SAVE)")
	cmd.Flags().BoolP("request", "", false, "Provide a raw HTTP request manually via STDIN in place of other options.")

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

	// Load Function
	// This path has gone through defaulting, env/flag overrieds and prompts.
	var f fn.Function
	if f, err = fn.NewFunction(cfg.Path); err != nil {
		return
	}

	// Message to send to the Function
	// Based on first static defaults, followed by overrides from flags/envs
	message := newInvokeMessage(cfg)

	// Invoke
	err = client.Invoke(cmd.Context(), f, message)
	if err != nil {
		return err
	}

	// Save
	// IF invoke completed without error (not necessarily successful invocation,
	// this means successful execution of an invocation attempt), save the final
	// value of the invocation settings such that future invocations can be run
	// without re-providing the values.
	// This is in essence updating the Function metadata to preserve default
	// invocation settings for this and other developers (settings are written
	// to func.yaml for commission to source control)
	if cfg.Save {
		err = f.Save()
	}

	// Confirm
	fmt.Fprintf(cmd.OutOrStderr(), "Invoked %v\n", f.Name)

	return
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

	cfg, err := newInvokeConfig(clientFn)
	failSoft(err)

	_ = clientFn(cfg) // TODO: remove if not used to create data for help tpl

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
	Confirm     bool
	Verbose     bool
}

func newInvokeConfig(clientFn invokeClientFn) (cfg invokeConfig, err error) {
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
		Confirm:     viper.GetBool("confirm"),
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
	fmt.Printf("Data: %v\n", cfg.Data)
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
				Default: c.Path,
			},
			Validate: func(val interface{}) error {
				derivedName, _ := deriveNameAndAbsolutePathFromPath(val.(string))
				return utils.ValidateFunctionName(derivedName)
			},
			Transform: func(ans interface{}) interface{} {
				_, absolutePath := deriveNameAndAbsolutePathFromPath(ans.(string))
				return absolutePath
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

	// Set a friendly default for target if it was not provided.
	if c.Target == "" {
		// If the functionis running locally,
		if client.Running(f) {
			c.Target = "local"
		} else if client.Deployed(f) {
			c.Target = "remote"
		}
	}
	qs = []*survey.Question{
		{
			Name: "Target",
			Prompt: &survey.Input{
				Message: "Target Function ('local', 'remote' or URL endpoint)",
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
