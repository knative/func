package cmd

import (
	"fmt"
	"io/ioutil"

	fn "github.com/boson-project/func"
	"github.com/boson-project/func/cloudevents"
	"github.com/boson-project/func/knative"
	"github.com/google/uuid"
	"github.com/ory/viper"
	"github.com/spf13/cobra"
)

func init() {
	e := cloudevents.NewEmitter()
	root.AddCommand(emitCmd)
	// TODO: do these env vars make sense?
	emitCmd.Flags().StringP("sink", "k", "", "Send the CloudEvent to the function running at [sink]. The special value \"local\" can be used to send the event to a function running on the local host. When provided, the --path flag is ignored  (Env: $FUNC_SINK)")
	emitCmd.Flags().StringP("source", "s", e.Source, "CloudEvent source (Env: $FUNC_SOURCE)")
	emitCmd.Flags().StringP("type", "t", e.Type, "CloudEvent type  (Env: $FUNC_TYPE)")
	emitCmd.Flags().StringP("id", "i", uuid.NewString(), "CloudEvent ID (Env: $FUNC_ID)")
	emitCmd.Flags().StringP("data", "d", "", "Any arbitrary string to be sent as the CloudEvent data. Ignored if --file is provided  (Env: $FUNC_DATA)")
	emitCmd.Flags().StringP("file", "f", "", "Path to a local file containing CloudEvent data to be sent  (Env: $FUNC_FILE)")
	emitCmd.Flags().StringP("content-type", "c", "application/json", "The MIME Content-Type for the CloudEvent data  (Env: $FUNC_CONTENT_TYPE)")
	emitCmd.Flags().StringP("path", "p", cwd(), "Path to the project directory. Ignored when --sink is provided (Env: $FUNC_PATH)")
}

var emitCmd = &cobra.Command{
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
	RunE:       runEmit,
}

func runEmit(cmd *cobra.Command, args []string) (err error) {
	config := newEmitConfig()
	var endpoint string
	if config.Sink != "" {
		if config.Sink == "local" {
			endpoint = "http://localhost:8080"
		} else {
			endpoint = config.Sink
		}
	} else {
		var f fn.Function
		f, err = fn.NewFunction(config.Path)
		if err != nil {
			return
		}
		// What happens if the function hasn't been deployed but they don't run with --local=true
		// Maybe we should be thinking about saving the endpoint URL in func.yaml after each deploy
		var d *knative.Describer
		d, err = knative.NewDescriber("")
		if err != nil {
			return
		}
		var desc fn.Description
		desc, err = d.Describe(f.Name)
		if err != nil {
			return
		}
		// Use the first available route
		endpoint = desc.Routes[0]
	}

	emitter := cloudevents.NewEmitter()
	emitter.Source = config.Source
	emitter.Type = config.Type
	emitter.Id = config.Id
	emitter.ContentType = config.ContentType
	emitter.Data = config.Data
	if config.File != "" {
		var buf []byte
		if emitter.Data != "" && config.Verbose {
			return fmt.Errorf("Only one of --data and --file may be specified \n")
		}
		buf, err = ioutil.ReadFile(config.File)
		if err != nil {
			return
		}
		emitter.Data = string(buf)
	}

	client := fn.New(
		fn.WithEmitter(emitter),
	)
	return client.Emit(cmd.Context(), endpoint)
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
