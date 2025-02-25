package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/utils"
)

func NewInvokeCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "invoke",
		Short: "Invoke a local or remote function",
		Long: `
NAME
	{{rootCmdUse}} invoke - test a function by invoking it with test data

SYNOPSIS
	{{rootCmdUse}} invoke [-t|--target] [-f|--format]
	             [--id] [--source] [--type] [--data] [--file] [--content-type]
	             [-s|--save] [-p|--path] [-i|--insecure] [-c|--confirm] [-v|--verbose]

DESCRIPTION
	Invokes the function by sending a test request to the currently running
	function instance, either locally or remote.  If the function is running
	both locally and remote, the local instance will be invoked.  This behavior
	can be manually overridden using the --target flag.

	Functions are invoked with a test data structure consisting of five values:
		id:            A unique identifier for the request.
		source:        A sender name for the request (sender).
		type:          A type for the request.
		data:          Data (content) for this request.
		content-type:  The MIME type of the value contained in 'data'.

	The values of these parameters can be individually altered from their defaults
	using their associated flags. Data can also be provided from a file using the
	--file flag.

	Invocation Target
	  The function instance to invoke can be specified using the --target flag
	  which accepts the values "local", "remote", or <URL>.  By default the
	  local function instance is chosen if running (see {{rootCmdUse}} run).
	  To explicitly target the remote (deployed) function:
	    {{rootCmdUse}} invoke --target=remote
	  To target an arbitrary endpoint, provide a URL:
	    {{rootCmdUse}} invoke --target=https://myfunction.example.com

	Invocation Data
	  Providing a filename in the --file flag will base64 encode its contents
	  as the "data" parameter sent to the function.  The value of --content-type
	  should be set to the type from the source file.  For example, the following
	  would send a JPEG base64 encoded in the "data" POST parameter:
	    {{rootCmdUse}} invoke --file=example.jpeg --content-type=image/jpeg

	Message Format
	  By default functions are sent messages which match the invocation format
	  of the template they were created using; for example "http" or "cloudevent".
	  To override this behavior, use the --format (-f) flag.
	    {{rootCmdUse}} invoke -f=cloudevent -t=http://my-sink.my-cluster

EXAMPLES

	o Invoke the default (local or remote) running function with default values
	  $ {{rootCmdUse}} invoke

	o Run the function locally and then invoke it with a test request:
	  (run in two terminals or by running the first in the background)
	  $ {{rootCmdUse}} run
	  $ {{rootCmdUse}} invoke

	o Deploy and then invoke the remote function:
	  $ {{rootCmdUse}} deploy
	  $ {{rootCmdUse}} invoke

	o Invoke a remote (deployed) function when it is already running locally:
	  (overrides the default behavior of preferring locally running instances)
	  $ {{rootCmdUse}} invoke --target=remote

	o Specify the data to send to the function as a flag
	  $ {{rootCmdUse}} invoke --data="Hello World!"

	o Send a JPEG to the function
	  $ {{rootCmdUse}} invoke --file=example.jpeg --content-type=image/jpeg

	o Invoke an arbitrary endpoint (HTTP POST)
		$ {{rootCmdUse}} invoke --target="https://my-http-handler.example.com"

	o Invoke an arbitrary endpoint (CloudEvent)
		$ {{rootCmdUse}} invoke -f=cloudevent -t="https://my-event-broker.example.com"

	o Allow insecure server connections when using SSL
		$ {{rootCmdUse}} invoke --insecure

`,
		SuggestFor: []string{"emit", "emti", "send", "emit", "exec", "nivoke", "onvoke", "unvoke", "knvoke", "imvoke", "ihvoke", "ibvoke"},
		PreRunE:    bindEnv("path", "format", "target", "id", "source", "type", "data", "content-type", "file", "insecure", "confirm", "verbose"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInvoke(cmd, args, newClient)
		},
	}

	// Config
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	// Flags
	cmd.Flags().StringP("format", "f", "", "Format of message to send, 'http' or 'cloudevent'.  Default is to choose automatically. ($FUNC_FORMAT)")
	cmd.Flags().StringP("target", "t", "", "Function instance to invoke.  Can be 'local', 'remote' or a URL.  Defaults to auto-discovery if not provided. ($FUNC_TARGET)")
	cmd.Flags().StringP("id", "", "", "ID for the request data. ($FUNC_ID)")
	cmd.Flags().StringP("source", "", fn.DefaultInvokeSource, "Source value for the request data. ($FUNC_SOURCE)")
	cmd.Flags().StringP("type", "", fn.DefaultInvokeType, "Type value for the request data. ($FUNC_TYPE)")
	cmd.Flags().StringP("content-type", "", fn.DefaultInvokeContentType, "Content Type of the data. ($FUNC_CONTENT_TYPE)")
	cmd.Flags().StringP("data", "", fn.DefaultInvokeData, "Data to send in the request. ($FUNC_DATA)")
	cmd.Flags().StringP("file", "", "", "Path to a file to use as data. Overrides --data flag and should be sent with a correct --content-type. ($FUNC_FILE)")
	cmd.Flags().BoolP("insecure", "i", false, "Allow insecure server connections when using SSL. ($FUNC_INSECURE)")
	addConfirmFlag(cmd, cfg.Confirm)
	addPathFlag(cmd)
	addVerboseFlag(cmd, cfg.Verbose)

	return cmd
}

// Run
func runInvoke(cmd *cobra.Command, _ []string, newClient ClientFactory) (err error) {
	// Gather flag values for the invocation
	cfg, err := newInvokeConfig()
	if err != nil {
		return
	}

	// Load the function
	f, err := fn.NewFunction(cfg.Path)
	if err != nil {
		return
	}
	if err = f.Validate(); err != nil {
		fmt.Printf("error validating function at '%v'. %v\n", f.Root, err)
		return err
	}
	if !f.Initialized() {
		return fn.NewErrNotInitialized(f.Root)
	}

	// Client instance from env vars, flags, args and user prompts (if --confirm)
	client, done := newClient(ClientConfig{Verbose: cfg.Verbose, InsecureSkipVerify: cfg.Insecure})
	defer done()

	// Message to send the running function built from parameters gathered
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
		m.Data = string(content)
	}

	// Invoke
	metadata, body, err := client.Invoke(cmd.Context(), cfg.Path, cfg.Target, m)
	if err != nil {
		return err
	}

	// When Verbose
	// - Print an explicit "Received response" indicator
	// - Print metadata (headers for HTTP requests, CloudEvents already include
	//   metadata in their data value.
	if cfg.Verbose {
		// Print a "Received response" message because a simple echo to
		// stdout could be confusing on a first-time run, viewing a proper echo.
		// user feedback suggests this actually be placed behind the --verbose
		// setting:
		fmt.Println("Function invoked.  Response:")

		if len(metadata) > 0 {
			fmt.Println("  Metadata:")
		}
		for k, vv := range metadata {
			values := strings.Join(vv, ";")
			fmt.Fprintf(cmd.OutOrStdout(), "    %v: %v\n", k, values)
		}
		if len(metadata) > 0 {
			fmt.Println("  Content:")
		}
	}

	// Always print the response's default stringification
	// Note body already includes a linebreak.
	fmt.Fprint(cmd.OutOrStdout(), body)
	return
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
	Confirm     bool
	Verbose     bool
	Insecure    bool
}

func newInvokeConfig() (cfg invokeConfig, err error) {
	cfg = invokeConfig{
		Path:        viper.GetString("path"),
		Target:      viper.GetString("target"),
		Format:      viper.GetString("format"),
		ID:          viper.GetString("id"),
		Source:      viper.GetString("source"),
		Type:        viper.GetString("type"),
		Data:        viper.GetString("data"),
		ContentType: viper.GetString("content-type"),
		File:        viper.GetString("file"),
		Confirm:     viper.GetBool("confirm"),
		Verbose:     viper.GetBool("verbose"),
		Insecure:    viper.GetBool("insecure"),
	}

	// If file was passed, read it in as data
	if cfg.File != "" {
		b, err := os.ReadFile(cfg.File)
		if err != nil {
			return cfg, err
		}
		cfg.Data = string(b)
	}

	// if not in confirm/prompting mode, the cfg structure is complete.
	if !cfg.Confirm {
		return
	}

	// If in interactive terminal mode, prompt to modify defaults.
	if interactiveTerminal() {
		return cfg.prompt()
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
	fmt.Printf("Insecure: %v\n", cfg.Insecure)
	return
}

func (c invokeConfig) prompt() (invokeConfig, error) {
	var qs []*survey.Question

	// First get path to effective function
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
	formatOptions := []string{"", "http", "cloudevent"}
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
				Options: formatOptions,
				Default: surveySelectDefault(c.Format, formatOptions),
			},
		},
	}
	if err := survey.Ask(qs, &c); err != nil {
		return c, err
	}

	// Prompt for the next set of values, with defaults set first by the function
	// as it exists on disk, followed by environment variables, and finally flags.
	// user interactive prompts therefore are the last applied, and thus highest
	// precedence values.
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

	qs = []*survey.Question{
		{
			Name: "Insecure",
			Prompt: &survey.Confirm{
				Message: "Allow insecure server connections when using SSL",
				Default: c.Insecure,
			},
		}}
	if err := survey.Ask(qs, &c); err != nil {
		return c, err
	}

	return c, nil
}
