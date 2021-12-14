package cmd

import (
	"context"
	"errors"
	"io/ioutil"
	"net/http"

	"github.com/google/uuid"
	"github.com/ory/viper"
	"github.com/spf13/cobra"
	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/cloudevents"
	fnhttp "knative.dev/kn-plugin-func/http"
	"knative.dev/kn-plugin-func/knative"
)

func init() {
	root.AddCommand(NewEmitCmd(newEmitClient))
}

// create a fn.Client with an instance of a
func newEmitClient(cfg emitConfig) (*fn.Client, error) {
	e := cloudevents.NewEmitter()
	e.Id = cfg.Id
	e.Source = cfg.Source
	e.Type = cfg.Type
	e.ContentType = cfg.ContentType
	e.Data = cfg.Data
	if e.Transport != nil {
		e.Transport = cfg.Transport
	}
	if cfg.File != "" {
		// See config.Validate for --Data and --file exclusivity enforcement
		b, err := ioutil.ReadFile(cfg.File)
		if err != nil {
			return nil, err
		}
		e.Data = string(b)
	}

	return fn.New(fn.WithEmitter(e)), nil
}

type emitClientFn func(emitConfig) (*fn.Client, error)

func NewEmitCmd(clientFn emitClientFn) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "emit",
		Short: "Emit a CloudEvent to a function endpoint",
		Long: `Emit event

Emits a CloudEvent, sending it to the deployed function.
`,
		Example: `
# Send a CloudEvent to the deployed function with no data and default values
# for source, type and ID
kn func emit

# Send a CloudEvent to the deployed function with the data found in ./test.json
kn func emit --file ./test.json

# Send a CloudEvent to the function running locally with a CloudEvent containing
# "Hello World!" as the data field, with a content type of "text/plain"
kn func emit --data "Hello World!" --content-type "text/plain" -s local

# Send a CloudEvent to the function running locally with an event type of "my.event"
kn func emit --type my.event --sink local

# Send a CloudEvent to the deployed function found at /path/to/fn with an id of "fn.test"
kn func emit --path /path/to/fn -i fn.test

# Send a CloudEvent to an arbitrary endpoint
kn func emit --sink "http://my.event.broker.com"
`,
		SuggestFor: []string{"meit", "emti", "send"},
		PreRunE:    bindEnv("source", "type", "id", "data", "file", "path", "sink", "content-type"),
	}

	cmd.Flags().StringP("sink", "k", "", "Send the CloudEvent to the function running at [sink]. The special value \"local\" can be used to send the event to a function running on the local host. When provided, the --path flag is ignored  (Env: $FUNC_SINK)")
	cmd.Flags().StringP("source", "s", cloudevents.DefaultSource, "CloudEvent source (Env: $FUNC_SOURCE)")
	cmd.Flags().StringP("type", "t", cloudevents.DefaultType, "CloudEvent type  (Env: $FUNC_TYPE)")
	cmd.Flags().StringP("id", "i", uuid.NewString(), "CloudEvent ID (Env: $FUNC_ID)")
	cmd.Flags().StringP("data", "d", "", "Any arbitrary string to be sent as the CloudEvent data. Ignored if --file is provided  (Env: $FUNC_DATA)")
	cmd.Flags().StringP("file", "f", "", "Path to a local file containing CloudEvent data to be sent  (Env: $FUNC_FILE)")
	cmd.Flags().StringP("content-type", "c", "application/json", "The MIME Content-Type for the CloudEvent data  (Env: $FUNC_CONTENT_TYPE)")
	setPathFlag(cmd)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runEmit(cmd, args, clientFn)
	}

	return cmd

}

func runEmit(cmd *cobra.Command, _ []string, clientFn emitClientFn) (err error) {
	config := newEmitConfig()

	// Validate things like invalid config combinations.
	if err := config.Validate(); err != nil {
		return err
	}

	// Determine the final endpoint, taking into account the special value "local",
	// and sampling the function's current route if not explicitly provided
	endpoint, err := endpoint(cmd.Context(), config)
	if err != nil {
		return err
	}

	// Instantiate a client based on the final value of config
	transport := fnhttp.NewRoundTripper()
	defer transport.Close()
	config.Transport = transport
	client, err := clientFn(config)
	if err != nil {
		return err
	}

	// Emit the event to the endpoint
	return client.Emit(cmd.Context(), endpoint)
}

// endpoint returns the final effective endpoint.
// By default, the contextually active Function is queried for it's current
// address (route).
// If "local" is specified in cfg.Sink, localhost is used.
// Otherwise the value of Sink is used verbatim if defined.
func endpoint(ctx context.Context, cfg emitConfig) (url string, err error) {
	var (
		f fn.Function
		d fn.Describer
		i fn.Info
	)

	// If the special value "local" was requested,
	// use localhost.
	if cfg.Sink == "local" {
		return "http://localhost:8080", nil
	}

	// If a sink was expressly provided, use that verbatim
	if cfg.Sink != "" {
		return cfg.Sink, nil
	}

	// If no sink was specified, use the route to the currently
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

type emitConfig struct {
	Path        string
	Source      string
	Type        string
	Id          string
	Data        string
	File        string
	ContentType string
	Sink        string
	Verbose     bool
	Transport   http.RoundTripper
}

func newEmitConfig() emitConfig {
	return emitConfig{
		Path:        viper.GetString("path"),
		Source:      viper.GetString("source"),
		Type:        viper.GetString("type"),
		Id:          viper.GetString("id"),
		Data:        viper.GetString("data"),
		File:        viper.GetString("file"),
		ContentType: viper.GetString("content-type"),
		Sink:        viper.GetString("sink"),
		Verbose:     viper.GetBool("verbose"),
	}
}

func (c emitConfig) Validate() error {
	if c.Data != "" && c.File != "" {
		return errors.New("Only one of --data or --file may be specified")
	}
	// TODO: should we verify that sink is a url or "local"?
	return nil
}
